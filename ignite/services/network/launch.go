package network

import (
	"context"
	"fmt"
	"time"

	launchtypes "github.com/tendermint/spn/x/launch/types"

	"github.com/ignite/cli/ignite/pkg/events"
	"github.com/ignite/cli/ignite/services/network/networktypes"
)

// MinLaunchTimeOffset represents an offset used when minimum launch time is used
// minimum launch time will be block time + minimum launch time duration param
// block time when tx is executed is not predicable, therefore we add few seconds
// to ensure the minimum duration is reached
const MinLaunchTimeOffset = time.Second * 30

// LaunchParams fetches the chain launch module params from SPN
func (n Network) LaunchParams(ctx context.Context) (launchtypes.Params, error) {
	res, err := n.launchQuery.Params(ctx, &launchtypes.QueryParamsRequest{})
	if err != nil {
		return launchtypes.Params{}, err
	}
	return res.GetParams(), nil
}

// TriggerLaunch launches a chain as a coordinator
func (n Network) TriggerLaunch(ctx context.Context, launchID uint64, launchTime time.Time) error {
	n.ev.Send(events.New(events.StatusOngoing, fmt.Sprintf("Launching chain %d", launchID)))
	params, err := n.LaunchParams(ctx)
	if err != nil {
		return err
	}

	var (
		minLaunchTime = n.clock.Now().Add(params.LaunchTimeRange.MinLaunchTime).Add(MinLaunchTimeOffset)
		maxLaunchTime = n.clock.Now().Add(params.LaunchTimeRange.MaxLaunchTime)
	)
	address, err := n.account.Address(networktypes.SPN)
	if err != nil {
		return err
	}

	if launchTime.IsZero() {
		// Use minimum launch time by default
		launchTime = minLaunchTime
	} else {
		// check launch time is in range
		switch {
		case launchTime.Before(minLaunchTime):
			return fmt.Errorf("launch time %s lower than minimum %s",
				launchTime.String(),
				minLaunchTime.String(),
			)
		case launchTime.After(maxLaunchTime):
			return fmt.Errorf("launch time %s bigger than maximum %s",
				launchTime.String(),
				maxLaunchTime.String(),
			)
		}
	}

	msg := launchtypes.NewMsgTriggerLaunch(address, launchID, launchTime)
	n.ev.Send(events.New(events.StatusOngoing, "Setting launch time"))
	res, err := n.cosmos.BroadcastTx(ctx, n.account, msg)
	if err != nil {
		return err
	}

	var launchRes launchtypes.MsgTriggerLaunchResponse
	if err := res.Decode(&launchRes); err != nil {
		return err
	}

	n.ev.Send(events.New(events.StatusDone,
		fmt.Sprintf("Chain %d will be launched on %s", launchID, launchTime.String()),
	))
	return nil
}

// RevertLaunch reverts a launched chain as a coordinator
func (n Network) RevertLaunch(ctx context.Context, launchID uint64, chain Chain) error {
	n.ev.Send(events.New(events.StatusOngoing, fmt.Sprintf("Reverting launched chain %d", launchID)))

	address, err := n.account.Address(networktypes.SPN)
	if err != nil {
		return err
	}

	msg := launchtypes.NewMsgRevertLaunch(address, launchID)
	_, err = n.cosmos.BroadcastTx(ctx, n.account, msg)
	if err != nil {
		return err
	}

	n.ev.Send(events.New(events.StatusDone,
		fmt.Sprintf("Chain %d launch was reverted", launchID),
	))

	n.ev.Send(events.New(events.StatusOngoing, "Resetting the genesis time"))
	if err := chain.ResetGenesisTime(); err != nil {
		return err
	}
	n.ev.Send(events.New(events.StatusDone, "Genesis time was reset"))
	return nil
}
