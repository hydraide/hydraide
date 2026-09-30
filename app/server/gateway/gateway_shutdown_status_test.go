package gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/hydraide/hydraide/app/core/hydra"
	hydrapb "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestHydraErrorStatus(t *testing.T) {
	t.Run("shutdown maps to Unavailable", func(t *testing.T) {
		err := hydraErrorStatus(errors.New(hydra.ErrorHydraIsShuttingDown))
		require.Equal(t, codes.Unavailable, status.Code(err))
	})
	t.Run("an existing status passes through unchanged", func(t *testing.T) {
		in := status.Error(codes.Unavailable, "swamp kept closing")
		require.Same(t, in, hydraErrorStatus(in))
	})
	t.Run("anything else maps to Internal", func(t *testing.T) {
		err := hydraErrorStatus(errors.New("disk on fire"))
		require.Equal(t, codes.Internal, status.Code(err))
		require.Contains(t, status.Convert(err).Message(), "disk on fire")
	})
}

type fakeEventsStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeEventsStream) Context() context.Context                      { return f.ctx }
func (f *fakeEventsStream) SendMsg(any) error                             { return nil }
func (f *fakeEventsStream) Send(*hydrapb.SubscribeToEventsResponse) error { return nil }

// TestGateway_HandlersAfterShutdownFlag_ReturnUnavailable covers the window in
// which a call has already passed the shutdown interceptor when hydra flips to
// shutting down: the handler must answer codes.Unavailable (retry elsewhere),
// not codes.Internal (server bug).
func TestGateway_HandlersAfterShutdownFlag_ReturnUnavailable(t *testing.T) {
	rig := newStreamVigilRig(t, "shutdown-status", "realm", "sw-1")
	rig.gw.ZeusInterface.GetHydra().MarkShuttingDown()

	value := "v"
	_, err := rig.gw.Set(context.Background(), &hydrapb.SetRequest{
		Swamps: []*hydrapb.SwampRequest{{
			IslandID:         rig.islandID,
			SwampName:        rig.swampName,
			KeyValues:        []*hydrapb.KeyValuePair{{Key: "k", StringVal: &value}},
			CreateIfNotExist: true,
			Overwrite:        true,
		}},
	})
	require.Error(t, err)
	require.Equal(t, codes.Unavailable, status.Code(err), "Set: %v", err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = rig.gw.SubscribeToEvents(&hydrapb.SubscribeToEventsRequest{
		IslandID:  rig.islandID,
		SwampName: rig.swampName,
	}, &fakeEventsStream{ctx: ctx})
	require.Error(t, err)
	require.Equal(t, codes.Unavailable, status.Code(err), "SubscribeToEvents: %v", err)
}
