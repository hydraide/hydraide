package client

import (
	"context"
	"testing"

	"github.com/hydraide/hydraide/generated/hydraidepbgo"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

// fakeHydraideServiceClient is a dummy implementation for test use only.
type fakeHydraideServiceClient struct {
	hydraidepbgo.HydraideServiceClient
}

func (f *fakeHydraideServiceClient) Heartbeat(
	_ context.Context,
	_ *hydraidepbgo.HeartbeatRequest,
	_ ...grpc.CallOption,
) (*hydraidepbgo.HeartbeatResponse, error) {
	return &hydraidepbgo.HeartbeatResponse{Pong: "beat"}, nil
}

func TestClient_GetServiceClient(t *testing.T) {
	// Arrange
	mockClient := &fakeHydraideServiceClient{}
	c := &client{
		allIslands: 1000,
		serviceClients: map[uint64]*ServiceClient{
			518: {
				GrpcClient: mockClient,
				Host:       "hydra01:4444",
			},
		},
	}

	swamp := name.New().Sanctuary("users").Realm("profiles").Swamp("john.doe")
	folder := swamp.GetIslandID(c.allIslands)

	// Act
	serviceClient := c.GetServiceClient(swamp)

	// Assert
	assert.NotNil(t, serviceClient)
	assert.Equal(t, mockClient, serviceClient)
	assert.Equal(t, uint64(518), folder, "Folder number mismatch – update map if needed")
}
