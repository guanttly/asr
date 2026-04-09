package user

import (
	"context"
	"errors"
	"testing"
	"time"

	userdomain "github.com/lgt/asr/internal/domain/user"
	wfdomain "github.com/lgt/asr/internal/domain/workflow"
)

type userRepoStub struct {
	user     *userdomain.User
	bindings *userdomain.WorkflowBindings
	saved    *userdomain.WorkflowBindings
}

func (r *userRepoStub) Create(_ context.Context, _ *userdomain.User) error { return nil }
func (r *userRepoStub) GetByID(_ context.Context, id uint64) (*userdomain.User, error) {
	if r.user == nil || r.user.ID != id {
		return nil, userdomain.ErrUserNotFound
	}
	return r.user, nil
}
func (r *userRepoStub) GetByUsername(_ context.Context, _ string) (*userdomain.User, error) {
	return nil, userdomain.ErrUserNotFound
}
func (r *userRepoStub) GetWorkflowBindings(_ context.Context, userID uint64) (*userdomain.WorkflowBindings, error) {
	if r.bindings != nil {
		return r.bindings, nil
	}
	return &userdomain.WorkflowBindings{UserID: userID}, nil
}
func (r *userRepoStub) SaveWorkflowBindings(_ context.Context, bindings *userdomain.WorkflowBindings) error {
	copy := *bindings
	copy.CreatedAt = time.Now()
	copy.UpdatedAt = copy.CreatedAt
	r.saved = &copy
	return nil
}
func (r *userRepoStub) Update(_ context.Context, _ *userdomain.User) error { return nil }
func (r *userRepoStub) Delete(_ context.Context, _ uint64) error           { return nil }
func (r *userRepoStub) List(_ context.Context, _, _ int) ([]*userdomain.User, int64, error) {
	return nil, 0, nil
}

type workflowRepoStub struct {
	items map[uint64]*wfdomain.Workflow
}

func (r *workflowRepoStub) Create(_ context.Context, _ *wfdomain.Workflow) error { return nil }
func (r *workflowRepoStub) GetByID(_ context.Context, id uint64) (*wfdomain.Workflow, error) {
	item, ok := r.items[id]
	if !ok {
		return nil, errors.New("workflow not found")
	}
	return item, nil
}
func (r *workflowRepoStub) Update(_ context.Context, _ *wfdomain.Workflow) error { return nil }
func (r *workflowRepoStub) Delete(_ context.Context, _ uint64) error             { return nil }
func (r *workflowRepoStub) List(_ context.Context, _ *wfdomain.OwnerType, _ *uint64, _ bool, _, _ int) ([]*wfdomain.Workflow, int64, error) {
	return nil, 0, nil
}
func (r *workflowRepoStub) ListFiltered(_ context.Context, _ *wfdomain.OwnerType, _ *uint64, _ bool, _ wfdomain.WorkflowListFilter, _, _ int) ([]*wfdomain.Workflow, int64, error) {
	return nil, 0, nil
}

func TestUpdateWorkflowBindingsPersistsValidatedBindings(t *testing.T) {
	userRepo := &userRepoStub{
		user: &userdomain.User{ID: 7, Role: userdomain.RoleUser},
	}
	workflowRepo := &workflowRepoStub{items: map[uint64]*wfdomain.Workflow{
		11: {ID: 11, WorkflowType: wfdomain.WorkflowTypeRealtime, OwnerType: wfdomain.OwnerUser, OwnerID: 7},
		12: {ID: 12, WorkflowType: wfdomain.WorkflowTypeBatch, OwnerType: wfdomain.OwnerSystem, IsPublished: true},
		13: {ID: 13, WorkflowType: wfdomain.WorkflowTypeMeeting, OwnerType: wfdomain.OwnerUser, OwnerID: 7},
	}}
	service := NewService(userRepo, workflowRepo)

	realtimeID := uint64(11)
	batchID := uint64(12)
	meetingID := uint64(13)
	result, err := service.UpdateWorkflowBindings(context.Background(), 7, &UpdateWorkflowBindingsRequest{
		Realtime: &realtimeID,
		Batch:    &batchID,
		Meeting:  &meetingID,
	})
	if err != nil {
		t.Fatalf("UpdateWorkflowBindings returned error: %v", err)
	}
	if result == nil || result.Realtime == nil || *result.Realtime != realtimeID {
		t.Fatalf("expected realtime binding to be saved, got %#v", result)
	}
	if userRepo.saved == nil || userRepo.saved.MeetingWorkflowID == nil || *userRepo.saved.MeetingWorkflowID != meetingID {
		t.Fatalf("expected repository save to receive meeting binding, got %#v", userRepo.saved)
	}
}

func TestUpdateWorkflowBindingsRejectsIncompatibleWorkflowType(t *testing.T) {
	userRepo := &userRepoStub{
		user: &userdomain.User{ID: 9, Role: userdomain.RoleUser},
	}
	workflowRepo := &workflowRepoStub{items: map[uint64]*wfdomain.Workflow{
		21: {ID: 21, WorkflowType: wfdomain.WorkflowTypeBatch, OwnerType: wfdomain.OwnerUser, OwnerID: 9},
	}}
	service := NewService(userRepo, workflowRepo)

	realtimeID := uint64(21)
	_, err := service.UpdateWorkflowBindings(context.Background(), 9, &UpdateWorkflowBindingsRequest{Realtime: &realtimeID})
	if err == nil {
		t.Fatal("expected incompatible workflow type to be rejected")
	}
	if userRepo.saved != nil {
		t.Fatalf("did not expect repository save on validation failure, got %#v", userRepo.saved)
	}
}
