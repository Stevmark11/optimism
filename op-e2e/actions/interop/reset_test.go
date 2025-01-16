package interop

import (
	"testing"

	"github.com/ethereum-optimism/optimism/op-e2e/actions/helpers"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum-optimism/optimism/op-node/rollup/event"
	"github.com/stretchr/testify/require"
)

func TestReset(gt *testing.T) {
	t := helpers.NewDefaultTesting(gt)

	is := SetupInterop(t)
	actors := is.CreateActors()

	// get both sequencers set up
	// sync the supervisor, handle initial events emitted by the nodes
	actors.ChainA.Sequencer.ActL2PipelineFull(t)
	actors.ChainA.Sequencer.SyncSupervisor(t)

	// No blocks yet
	status := actors.ChainA.Sequencer.SyncStatus()
	require.Equal(t, uint64(0), status.UnsafeL2.Number)

	// Sync initial Supervisor state
	actors.Supervisor.ProcessFull(t)

	// Advance the chain by one block, and step through the sync process
	// until the block is cross safe.
	currentBlockNum := 0
	advanceChainA := func() {
		currentBlockNum++
		prevBlockNum := currentBlockNum - 1
		// Build L2 block on chain A
		actors.ChainA.Sequencer.ActL2StartBlock(t)
		actors.ChainA.Sequencer.ActL2EndBlock(t)
		status = actors.ChainA.Sequencer.SyncStatus()
		head := status.UnsafeL2.ID()
		require.Equal(t, uint64(currentBlockNum), head.Number)
		require.Equal(t, uint64(prevBlockNum), status.CrossUnsafeL2.Number)
		require.Equal(t, uint64(prevBlockNum), status.LocalSafeL2.Number)
		require.Equal(t, uint64(prevBlockNum), status.SafeL2.Number)

		// Ingest the new unsafe-block event
		actors.ChainA.Sequencer.SyncSupervisor(t)

		// Verify as cross-unsafe with supervisor
		actors.Supervisor.ProcessFull(t)
		actors.ChainA.Sequencer.ActL2PipelineFull(t)
		status = actors.ChainA.Sequencer.SyncStatus()
		require.Equal(t, head, status.UnsafeL2.ID())
		require.Equal(t, head, status.CrossUnsafeL2.ID())
		require.Equal(t, uint64(prevBlockNum), status.LocalSafeL2.Number)
		require.Equal(t, uint64(prevBlockNum), status.SafeL2.Number)

		// Submit the L2 block, sync the local-safe data
		actors.ChainA.Batcher.ActSubmitAll(t)
		actors.L1Miner.ActL1StartBlock(12)(t)
		actors.L1Miner.ActL1IncludeTx(actors.ChainA.BatcherAddr)(t)
		actors.L1Miner.ActL1EndBlock(t)

		// The node will exhaust L1 data,
		// it needs the supervisor to see the L1 block first,
		// and provide it to the node.
		actors.ChainA.Sequencer.ActL2EventsUntil(t, event.Is[derive.ExhaustedL1Event], 100, false)
		actors.Supervisor.SignalLatestL1(t)          // supervisor will be aware of latest L1
		actors.ChainA.Sequencer.SyncSupervisor(t)    // supervisor to react to exhaust-L1
		actors.ChainA.Sequencer.ActL2PipelineFull(t) // node to complete syncing to L1 head.

		actors.ChainA.Sequencer.ActL1HeadSignal(t) // TODO: two sources of L1 head
		status = actors.ChainA.Sequencer.SyncStatus()
		require.Equal(t, head, status.UnsafeL2.ID())
		require.Equal(t, head, status.CrossUnsafeL2.ID())
		require.Equal(t, head, status.LocalSafeL2.ID())
		require.Equal(t, uint64(prevBlockNum), status.SafeL2.Number)
		// Local-safe does not count as "safe" in RPC
		n := actors.ChainA.SequencerEngine.L2Chain().CurrentSafeBlock().Number.Uint64()
		require.Equal(t, uint64(prevBlockNum), n)

		// Make the supervisor aware of the new L1 block
		actors.Supervisor.SignalLatestL1(t)

		// Ingest the new local-safe event
		actors.ChainA.Sequencer.SyncSupervisor(t)

		// Cross-safe verify it
		actors.Supervisor.ProcessFull(t)
		actors.ChainA.Sequencer.ActL2PipelineFull(t)
		status = actors.ChainA.Sequencer.SyncStatus()
		require.Equal(t, head, status.UnsafeL2.ID())
		require.Equal(t, head, status.CrossUnsafeL2.ID())
		require.Equal(t, head, status.LocalSafeL2.ID())
		require.Equal(t, head, status.SafeL2.ID())
		h := actors.ChainA.SequencerEngine.L2Chain().CurrentSafeBlock().Hash()
		require.Equal(t, head.Hash, h)
	}

	howMany := 10
	// Advance through multiple blocks
	for i := 0; i < howMany; i++ {
		advanceChainA()
	}

	// Reset the supervisor to not have the
	actors.Supervisor.backend.Reset(actors.ChainA.ChainID, uint64(howMany-2))
	actors.Supervisor.ProcessFull(t)
	actors.ChainA.Sequencer.ActL2PipelineFull(t)

	// Advance the chain again, and we should see this test fail
	// because the next block which would be built after this reset should be howMany - 1 (8)
	// but the requires in advanceChainA() expect the block number to be currentBlockNum + 1 (11)
	advanceChainA()
}
