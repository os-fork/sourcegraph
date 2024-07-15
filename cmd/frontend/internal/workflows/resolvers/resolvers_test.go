package resolvers

import (
	"context"
	"reflect"
	"testing"

	mockrequire "github.com/derision-test/go-mockgen/v2/testutil/require"
	"github.com/graph-gophers/graphql-go"
	"github.com/stretchr/testify/require"

	"github.com/sourcegraph/sourcegraph/cmd/frontend/graphqlbackend"
	"github.com/sourcegraph/sourcegraph/internal/actor"
	"github.com/sourcegraph/sourcegraph/internal/auth"
	"github.com/sourcegraph/sourcegraph/internal/database"
	"github.com/sourcegraph/sourcegraph/internal/database/dbmocks"
	"github.com/sourcegraph/sourcegraph/internal/types"
)

func TestWorkflows(t *testing.T) {
	userID := int32(1)
	users := dbmocks.NewMockUserStore()
	users.GetByIDFunc.SetDefaultReturn(&types.User{SiteAdmin: true, ID: userID}, nil)

	workflows := dbmocks.NewMockWorkflowStore()
	workflows.ListFunc.SetDefaultHook(func(_ context.Context, args database.WorkflowListArgs, paginationArgs *database.PaginationArgs) ([]*types.Workflow, error) {
		return []*types.Workflow{{ID: userID, Name: "n", Owner: *args.Owner}}, nil
	})
	workflows.CountFunc.SetDefaultHook(func(_ context.Context, args database.WorkflowListArgs) (int, error) {
		return 1, nil
	})

	db := dbmocks.NewMockDB()
	db.UsersFunc.SetDefaultReturn(users)
	db.WorkflowsFunc.SetDefaultReturn(workflows)

	ownerID := graphqlbackend.MarshalUserID(userID)
	args := graphqlbackend.WorkflowsArgs{
		Owner:                  &ownerID,
		OrderBy:                graphqlbackend.WorkflowsOrderByUpdatedAt,
		ConnectionResolverArgs: dummyConnectionResolverArgs,
	}

	resolver, err := newTestResolver(t, db).Workflows(actor.WithActor(context.Background(), actor.FromUser(userID)), args)
	if err != nil {
		t.Fatal(err)
	}

	nodes, err := resolver.Nodes(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	wantNodes := []graphqlbackend.WorkflowResolver{
		&workflowResolver{db, types.Workflow{
			ID:    userID,
			Name:  "n",
			Owner: types.NamespaceUser(userID),
		}},
	}
	if !reflect.DeepEqual(nodes, wantNodes) {
		t.Errorf("got %v+, want %v+", nodes[0], wantNodes[0])
	}
}

func TestWorkflowsForSameUser(t *testing.T) {
	userID := int32(1)
	users := dbmocks.NewMockUserStore()
	users.GetByIDFunc.SetDefaultReturn(&types.User{ID: userID}, nil)

	workflows := dbmocks.NewMockWorkflowStore()
	workflows.ListFunc.SetDefaultHook(func(_ context.Context, args database.WorkflowListArgs, paginationArgs *database.PaginationArgs) ([]*types.Workflow, error) {
		return []*types.Workflow{{ID: 1, Name: "n", Owner: *args.Owner}}, nil
	})
	workflows.CountFunc.SetDefaultHook(func(_ context.Context, args database.WorkflowListArgs) (int, error) {
		return 1, nil
	})

	db := dbmocks.NewMockDB()
	db.UsersFunc.SetDefaultReturn(users)
	db.WorkflowsFunc.SetDefaultReturn(workflows)

	ownerID := graphqlbackend.MarshalUserID(userID)
	args := graphqlbackend.WorkflowsArgs{
		Owner:                  &ownerID,
		OrderBy:                graphqlbackend.WorkflowsOrderByUpdatedAt,
		ConnectionResolverArgs: dummyConnectionResolverArgs,
	}

	resolver, err := newTestResolver(t, db).Workflows(actor.WithActor(context.Background(), actor.FromUser(userID)), args)
	if err != nil {
		t.Fatal(err)
	}

	nodes, err := resolver.Nodes(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	wantNodes := []graphqlbackend.WorkflowResolver{
		&workflowResolver{db, types.Workflow{
			ID:    1,
			Name:  "n",
			Owner: types.NamespaceUser(userID),
		}},
	}
	if !reflect.DeepEqual(nodes, wantNodes) {
		t.Errorf("got %v+, want %v+", nodes[0], wantNodes[0])
	}
}

func TestWorkflowsForDifferentUser(t *testing.T) {
	userID := int32(2)
	users := dbmocks.NewMockUserStore()
	users.GetByIDFunc.SetDefaultReturn(&types.User{ID: userID}, nil)

	workflows := dbmocks.NewMockWorkflowStore()
	workflows.ListFunc.SetDefaultHook(func(_ context.Context, args database.WorkflowListArgs, paginationArgs *database.PaginationArgs) ([]*types.Workflow, error) {
		panic("should fail auth check and never be called")
	})
	workflows.CountFunc.SetDefaultHook(func(_ context.Context, args database.WorkflowListArgs) (int, error) {
		panic("should fail auth check and never be called")
	})

	db := dbmocks.NewMockDB()
	db.UsersFunc.SetDefaultReturn(users)
	db.WorkflowsFunc.SetDefaultReturn(workflows)

	ownerID := graphqlbackend.MarshalUserID(3)
	args := graphqlbackend.WorkflowsArgs{
		Owner:                  &ownerID,
		OrderBy:                graphqlbackend.WorkflowsOrderByUpdatedAt,
		ConnectionResolverArgs: dummyConnectionResolverArgs,
	}

	_, err := newTestResolver(t, db).Workflows(actor.WithActor(context.Background(), actor.FromUser(userID)), args)
	if err == nil {
		t.Error("got nil, want error to be returned for accessing workflows of different user by non-site admin")
	}
}

func TestWorkflowsForDifferentOrg(t *testing.T) {
	userID := int32(1)
	users := dbmocks.NewMockUserStore()
	users.GetByIDFunc.SetDefaultReturn(&types.User{ID: userID}, nil)
	users.GetByCurrentAuthUserFunc.SetDefaultReturn(&types.User{ID: userID}, nil)

	orgID := int32(2)
	om := dbmocks.NewMockOrgMemberStore()
	om.GetByOrgIDAndUserIDFunc.SetDefaultHook(func(ctx context.Context, oid, uid int32) (*types.OrgMembership, error) {
		return nil, nil
	})

	workflows := dbmocks.NewMockWorkflowStore()
	workflows.ListFunc.SetDefaultHook(func(_ context.Context, args database.WorkflowListArgs, paginationArgs *database.PaginationArgs) ([]*types.Workflow, error) {
		return []*types.Workflow{{ID: 1, Name: "n", Owner: types.NamespaceOrg(orgID)}}, nil
	})
	workflows.CountFunc.SetDefaultHook(func(_ context.Context, args database.WorkflowListArgs) (int, error) {
		return 1, nil
	})

	db := dbmocks.NewMockDB()
	db.UsersFunc.SetDefaultReturn(users)
	db.OrgMembersFunc.SetDefaultReturn(om)
	db.WorkflowsFunc.SetDefaultReturn(workflows)

	ownerID := graphqlbackend.MarshalOrgID(orgID)
	args := graphqlbackend.WorkflowsArgs{
		Owner:                  &ownerID,
		OrderBy:                graphqlbackend.WorkflowsOrderByUpdatedAt,
		ConnectionResolverArgs: dummyConnectionResolverArgs,
	}

	if _, err := newTestResolver(t, db).Workflows(actor.WithActor(context.Background(), actor.FromUser(userID)), args); err != auth.ErrNotAnOrgMember {
		t.Errorf("got %v+, want %v+", err, auth.ErrNotAnOrgMember)
	}
}

func TestWorkflowByIDOwner(t *testing.T) {
	ctx := context.Background()

	userID := int32(1)
	fixtureID := int32(1)

	users := dbmocks.NewMockUserStore()
	users.GetByCurrentAuthUserFunc.SetDefaultReturn(&types.User{ID: userID}, nil)

	workflows := dbmocks.NewMockWorkflowStore()
	workflows.GetByIDFunc.SetDefaultReturn(
		&types.Workflow{
			ID:    fixtureID,
			Name:  "n",
			Owner: types.NamespaceUser(userID),
		},
		nil,
	)

	db := dbmocks.NewMockDB()
	db.UsersFunc.SetDefaultReturn(users)
	db.WorkflowsFunc.SetDefaultReturn(workflows)

	ctx = actor.WithActor(ctx, &actor.Actor{UID: userID})

	workflow, err := newTestResolver(t, db).WorkflowByID(ctx, marshalWorkflowID(fixtureID))
	if err != nil {
		t.Fatal(err)
	}
	want := &workflowResolver{
		db: db,
		s: types.Workflow{
			ID:    fixtureID,
			Name:  "n",
			Owner: types.NamespaceUser(userID),
		},
	}

	if !reflect.DeepEqual(workflow, want) {
		t.Errorf("got %v+, want %v+", workflow, want)
	}
}

func TestWorkflowByIDNonOwner(t *testing.T) {
	// Non-owners cannot view a user's workflows.
	userID := int32(1)
	otherUserID := int32(2)
	fixtureID := marshalWorkflowID(1)

	users := dbmocks.NewMockUserStore()
	users.GetByCurrentAuthUserFunc.SetDefaultReturn(&types.User{SiteAdmin: true, ID: otherUserID}, nil)

	workflows := dbmocks.NewMockWorkflowStore()
	workflows.GetByIDFunc.SetDefaultReturn(
		&types.Workflow{
			Name:  "n",
			Owner: types.NamespaceUser(userID),
		},
		nil,
	)

	db := dbmocks.NewMockDB()
	db.UsersFunc.SetDefaultReturn(users)
	db.WorkflowsFunc.SetDefaultReturn(workflows)

	ctx := actor.WithActor(context.Background(), &actor.Actor{UID: otherUserID})

	_, err := newTestResolver(t, db).WorkflowByID(ctx, fixtureID)
	t.Log(err)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestCreateWorkflow(t *testing.T) {
	userID := int32(1)
	users := dbmocks.NewMockUserStore()
	users.GetByCurrentAuthUserFunc.SetDefaultReturn(&types.User{SiteAdmin: true, ID: userID}, nil)

	ctx := actor.WithActor(context.Background(), &actor.Actor{UID: userID})

	workflowStore := dbmocks.NewMockWorkflowStore()
	workflowStore.CreateFunc.SetDefaultHook(func(_ context.Context, newWorkflow *types.Workflow, actorUID int32) (*types.Workflow, error) {
		return &types.Workflow{
			ID:            1,
			Name:          newWorkflow.Name,
			Description:   newWorkflow.Description,
			Owner:         newWorkflow.Owner,
			TemplateText:  newWorkflow.TemplateText,
			Draft:         newWorkflow.Draft,
			CreatedByUser: &actorUID,
			UpdatedByUser: &actorUID,
		}, nil
	})

	db := dbmocks.NewMockDB()
	db.UsersFunc.SetDefaultReturn(users)
	db.WorkflowsFunc.SetDefaultReturn(workflowStore)

	workflows, err := newTestResolver(t, db).CreateWorkflow(ctx, &graphqlbackend.CreateWorkflowArgs{
		Input: graphqlbackend.WorkflowInput{
			Name:  "n",
			Draft: true,
			Owner: graphqlbackend.MarshalUserID(userID),
		}})
	if err != nil {
		t.Fatal(err)
	}
	want := &workflowResolver{db, types.Workflow{
		ID:            1,
		Name:          "n",
		Draft:         true,
		Owner:         types.NamespaceUser(userID),
		CreatedByUser: &userID,
		UpdatedByUser: &userID,
	}}

	mockrequire.Called(t, workflowStore.CreateFunc)

	if !reflect.DeepEqual(workflows, want) {
		t.Errorf("got %v+, want %v+", workflows, want)
	}
}

func TestUpdateWorkflow(t *testing.T) {
	fixtureID := int32(1)
	userID := int32(1)
	users := dbmocks.NewMockUserStore()
	users.GetByCurrentAuthUserFunc.SetDefaultReturn(&types.User{SiteAdmin: true, ID: userID}, nil)

	ctx := actor.WithActor(context.Background(), &actor.Actor{UID: userID})

	workflowStore := dbmocks.NewMockWorkflowStore()
	workflowStore.UpdateFunc.SetDefaultHook(func(ctx context.Context, workflow *types.Workflow, actorUID int32) (*types.Workflow, error) {
		return &types.Workflow{
			ID:            fixtureID,
			Name:          workflow.Name,
			Description:   workflow.Description,
			Owner:         workflow.Owner,
			TemplateText:  workflow.TemplateText,
			Draft:         workflow.Draft,
			CreatedByUser: &actorUID,
			UpdatedByUser: &actorUID,
		}, nil
	})
	workflowStore.GetByIDFunc.SetDefaultReturn(&types.Workflow{Owner: types.NamespaceUser(userID)}, nil)

	db := dbmocks.NewMockDB()
	db.UsersFunc.SetDefaultReturn(users)
	db.WorkflowsFunc.SetDefaultReturn(workflowStore)

	workflows, err := newTestResolver(t, db).UpdateWorkflow(ctx, &graphqlbackend.UpdateWorkflowArgs{
		ID: marshalWorkflowID(fixtureID),
		Input: graphqlbackend.WorkflowUpdateInput{
			Name:  "n2",
			Draft: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := &workflowResolver{db, types.Workflow{
		ID:            fixtureID,
		Name:          "n2",
		Draft:         true,
		Owner:         types.NamespaceUser(userID),
		CreatedByUser: &userID,
		UpdatedByUser: &userID,
	}}

	mockrequire.Called(t, workflowStore.UpdateFunc)

	if !reflect.DeepEqual(workflows, want) {
		t.Errorf("got %v+, want %v+", workflows, want)
	}
}

func TestTransferWorkflowOwnership(t *testing.T) {
	fixtureID := int32(1)
	userID := int32(1)
	users := dbmocks.NewMockUserStore()
	users.GetByCurrentAuthUserFunc.SetDefaultReturn(&types.User{ID: userID}, nil)

	ctx := actor.WithActor(context.Background(), &actor.Actor{UID: userID})

	orgID := int32(2)
	om := dbmocks.NewMockOrgMemberStore()
	om.GetByOrgIDAndUserIDFunc.SetDefaultHook(func(ctx context.Context, oid, uid int32) (*types.OrgMembership, error) {
		return &types.OrgMembership{OrgID: oid, UserID: uid}, nil
	})

	workflows := dbmocks.NewMockWorkflowStore()
	workflows.UpdateOwnerFunc.SetDefaultHook(func(ctx context.Context, id int32, newOwner types.Namespace, actorUID int32) (*types.Workflow, error) {
		return &types.Workflow{
			ID:    id,
			Owner: newOwner,
		}, nil
	})
	workflows.GetByIDFunc.SetDefaultReturn(&types.Workflow{ID: fixtureID, Owner: types.NamespaceUser(userID)}, nil)

	db := dbmocks.NewMockDB()
	db.UsersFunc.SetDefaultReturn(users)
	db.OrgMembersFunc.SetDefaultReturn(om)
	db.WorkflowsFunc.SetDefaultReturn(workflows)

	result, err := newTestResolver(t, db).TransferWorkflowOwnership(ctx, &graphqlbackend.TransferWorkflowOwnershipArgs{
		ID:       marshalWorkflowID(fixtureID),
		NewOwner: graphqlbackend.MarshalOrgID(orgID),
	})
	if err != nil {
		t.Fatal(err)
	}

	mockrequire.Called(t, workflows.UpdateOwnerFunc)
	mockrequire.Called(t, om.GetByOrgIDAndUserIDFunc)
	want := &workflowResolver{db, types.Workflow{
		ID:    fixtureID,
		Owner: types.NamespaceOrg(orgID),
	}}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("got %v+, want %v+", result, want)
	}
}

func TestUpdateWorkflowPermissions(t *testing.T) {
	user1 := &types.User{ID: 42}
	user2 := &types.User{ID: 43}
	admin := &types.User{ID: 44, SiteAdmin: true}
	org1 := &types.Org{ID: 42}
	org2 := &types.Org{ID: 43}

	cases := []struct {
		execUser *types.User
		ssUserID *int32
		ssOrgID  *int32
		errIs    error
	}{{
		execUser: user1,
		ssUserID: &user1.ID,
		errIs:    nil,
	}, {
		execUser: user1,
		ssUserID: &user2.ID,
		errIs:    &auth.InsufficientAuthorizationError{},
	}, {
		execUser: user1,
		ssOrgID:  &org1.ID,
		errIs:    nil,
	}, {
		execUser: user1,
		ssOrgID:  &org2.ID,
		errIs:    auth.ErrNotAnOrgMember,
	}, {
		execUser: admin,
		ssOrgID:  &user1.ID,
		errIs:    nil,
	}, {
		execUser: admin,
		ssOrgID:  &org1.ID,
		errIs:    nil,
	}}

	for _, tt := range cases {
		t.Run("", func(t *testing.T) {
			ctx := actor.WithActor(context.Background(), actor.FromUser(tt.execUser.ID))
			users := dbmocks.NewMockUserStore()
			users.GetByCurrentAuthUserFunc.SetDefaultHook(func(ctx context.Context) (*types.User, error) {
				switch actor.FromContext(ctx).UID {
				case user1.ID:
					return user1, nil
				case user2.ID:
					return user2, nil
				case admin.ID:
					return admin, nil
				default:
					panic("bad actor")
				}
			})

			workflows := dbmocks.NewMockWorkflowStore()
			workflows.UpdateFunc.SetDefaultHook(func(_ context.Context, ss *types.Workflow, actorUID int32) (*types.Workflow, error) {
				return ss, nil
			})
			workflows.GetByIDFunc.SetDefaultReturn(&types.Workflow{
				Owner: types.Namespace{
					User: tt.ssUserID,
					Org:  tt.ssOrgID,
				},
			}, nil)

			orgMembers := dbmocks.NewMockOrgMemberStore()
			orgMembers.GetByOrgIDAndUserIDFunc.SetDefaultHook(func(_ context.Context, orgID int32, userID int32) (*types.OrgMembership, error) {
				if orgID == userID {
					return &types.OrgMembership{}, nil
				}
				return nil, nil
			})

			db := dbmocks.NewMockDB()
			db.UsersFunc.SetDefaultReturn(users)
			db.WorkflowsFunc.SetDefaultReturn(workflows)
			db.OrgMembersFunc.SetDefaultReturn(orgMembers)

			_, err := newTestResolver(t, db).UpdateWorkflow(ctx, &graphqlbackend.UpdateWorkflowArgs{
				ID: marshalWorkflowID(1),
				Input: graphqlbackend.WorkflowUpdateInput{
					Name: "n2",
				},
			})
			if tt.errIs == nil {
				require.NoError(t, err)
			} else {
				require.ErrorAs(t, err, &tt.errIs)
			}
		})
	}
}

func TestDeleteWorkflow(t *testing.T) {
	userID := int32(1)
	users := dbmocks.NewMockUserStore()
	users.GetByIDFunc.SetDefaultReturn(&types.User{SiteAdmin: true, ID: userID}, nil)

	ctx := actor.WithActor(context.Background(), &actor.Actor{UID: userID})

	w := dbmocks.NewMockWorkflowStore()
	w.GetByIDFunc.SetDefaultReturn(&types.Workflow{
		ID:    1,
		Name:  "n",
		Owner: types.NamespaceUser(userID),
	}, nil)

	w.DeleteFunc.SetDefaultReturn(nil)

	db := dbmocks.NewMockDB()
	db.UsersFunc.SetDefaultReturn(users)
	db.WorkflowsFunc.SetDefaultReturn(w)

	firstWorkflowGraphqlID := graphql.ID("U2F2ZWRTZWFyY2g6NTI=")
	_, err := newTestResolver(t, db).DeleteWorkflow(ctx, &graphqlbackend.DeleteWorkflowArgs{ID: firstWorkflowGraphqlID})
	if err != nil {
		t.Fatal(err)
	}

	mockrequire.Called(t, w.DeleteFunc)
}

func newTestResolver(t *testing.T, db database.DB) *Resolver {
	t.Helper()
	return &Resolver{db: db}
}
