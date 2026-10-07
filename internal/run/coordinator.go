package run

import (
	"context"
	"fmt"
	"github.com/wyw14/cry-161/internal/chamber"
	"github.com/wyw14/cry-161/internal/journal"
	"github.com/wyw14/cry-161/internal/model"
	"github.com/wyw14/cry-161/internal/recipe"
	"github.com/wyw14/cry-161/internal/substrate"
	"github.com/wyw14/cry-161/internal/vacuum"
	"time"
)

type Coordinator struct {
	Registry *Registry
	Recipes  *recipe.Store
	Carrier  *substrate.Carrier
	Link     *substrate.Link
	Chamber  *chamber.Chamber
	Vacuum   *vacuum.Controller
	Journal  *journal.Store
}

func (c *Coordinator) Start(ctx context.Context, carrierID, batch string, recipeID model.ID) (*model.Run, error) {
	if carrierID == "" {
		carrierID = "carrier-" + batch
	}
	if _, err := c.Recipes.Active(recipeID); err != nil {
		return nil, err
	}
	run := &model.Run{ID: model.NewID(), Batch: batch, RecipeID: recipeID, State: model.Loading, CreatedAt: timeNow(), UpdatedAt: timeNow()}
	if err := c.Carrier.Load(batch, run.ID); err != nil {
		return nil, err
	}
	if c.Link != nil {
		if _, err := c.Link.LoadBatch(carrierID, batch, "load_port", "system"); err != nil {
			c.Carrier.Unload(batch)
			return nil, err
		}
		if _, err := c.Link.BindRun(carrierID, run.ID, "load_port", "system"); err != nil {
			c.Carrier.Unload(batch)
			return nil, err
		}
	}
	c.Registry.Add(run)
	if !c.Registry.Transition(run.ID, model.Pumping) {
		return nil, fmt.Errorf("state transition failed")
	}
	if err := c.Vacuum.Start(ctx); err != nil {
		c.Registry.UpdateError(run.ID, err)
		return nil, err
	}
	c.Chamber.SetVacuum(c.Vacuum.Pressure(), c.Vacuum.Stable())
	c.Chamber.SetInterlock("vacuum", true)
	_ = c.Journal.Append(journal.RunStarted(*run))
	value, _ := c.Registry.Get(run.ID)
	return value, nil
}
func timeNow() time.Time { return time.Now().UTC() }
