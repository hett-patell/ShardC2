package transport

import (
	"context"
	"testing"
)

type mockClient struct {
	registerCalled    bool
	beaconCalled      bool
	getCommandsCalled bool
	submitCalled      bool
}

func (m *mockClient) Register(ctx context.Context, req RegisterRequest) (RegisterResponse, error) {
	m.registerCalled = true
	return RegisterResponse{ID: "test-id", SessionToken: "test-token"}, nil
}

func (m *mockClient) Beacon(ctx context.Context, req BeaconRequest) (BeaconResponse, error) {
	m.beaconCalled = true
	return BeaconResponse{Status: "ok", PendingCommands: 0}, nil
}

func (m *mockClient) GetCommands(ctx context.Context) ([]Command, error) {
	m.getCommandsCalled = true
	return nil, nil
}

func (m *mockClient) SubmitResult(ctx context.Context, result CommandResult) error {
	m.submitCalled = true
	return nil
}

func TestClientInterfaceIsImplementable(t *testing.T) {
	var c Client = &mockClient{}

	resp, err := c.Register(context.Background(), RegisterRequest{Hostname: "test"})
	if err != nil || resp.ID != "test-id" {
		t.Fatalf("register: %v, %+v", err, resp)
	}

	beacon, err := c.Beacon(context.Background(), BeaconRequest{BotID: "test-id"})
	if err != nil || beacon.Status != "ok" {
		t.Fatalf("beacon: %v, %+v", err, beacon)
	}

	cmds, err := c.GetCommands(context.Background())
	if err != nil || cmds != nil {
		t.Fatalf("commands: %v, %+v", err, cmds)
	}

	if err := c.SubmitResult(context.Background(), CommandResult{CommandID: "cmd-1", Output: "ok", Status: "completed"}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	m := c.(*mockClient)
	if !m.registerCalled || !m.beaconCalled || !m.getCommandsCalled || !m.submitCalled {
		t.Fatal("not all interface methods were called")
	}
}
