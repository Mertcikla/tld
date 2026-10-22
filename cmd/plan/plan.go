// Package plan exposes recorded plans through the same local/remote Connect API.
package plan

import (
	"fmt"
	"io"
	"os"

	rpc "buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/pkg/plans"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func NewCmd(wdir *string) *cobra.Command {
	root := &cobra.Command{Use: "plan", Short: "Create, scope, and review workspace change plans"}
	withClient := func(fn func(*cobra.Command, []string, rpc.PlanServiceClient) error) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) error {
			session, err := cmdutil.OpenSession(cmd, *wdir, "", "")
			if err != nil {
				return err
			}
			defer func() { _ = session.Close() }()
			target, dir := cmdutil.TargetOverrides(cmd, "", "")
			c, _, closeFn, err := exec.PlanClients(session.Config, target, dir, "")
			if err != nil {
				return err
			}
			defer func() { _ = closeFn() }()
			if len(args) > 0 && cmd.Name() != "new" && cmd.Name() != "list" {
				p, e := c.GetPlan(cmd.Context(), connect.NewRequest(&pb.GetPlanRequest{PlanId: args[0]}))
				if e != nil {
					return e
				}
				args = append([]string(nil), args...)
				args[0] = p.Msg.Plan.Id
				if cmd.Name() == "include" || cmd.Name() == "exclude" || cmd.Name() == "note" {
					key, e := resolveResource(cmd, c, args[0], args[1], cmd.Name() == "note")
					if e != nil {
						return e
					}
					args[1] = key
				}
			}
			return fn(cmd, args, c)
		}
	}
	print := func(cmd *cobra.Command, m proto.Message) error {
		b, err := protojson.MarshalOptions{Multiline: true, UseProtoNames: true}.Marshal(m)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(b))
		return err
	}
	var repo, name, objective string
	var resume bool
	create := &cobra.Command{Use: "new", Short: "Create or resume a draft", Args: cobra.NoArgs, RunE: withClient(func(cmd *cobra.Command, _ []string, c rpc.PlanServiceClient) error {
		r, e := c.CreatePlan(cmd.Context(), connect.NewRequest(&pb.CreatePlanRequest{RepositoryId: repo, Name: name, Objective: objective, Resume: resume}))
		if e != nil {
			return e
		}
		return print(cmd, r.Msg)
	})}
	create.Flags().StringVar(&repo, "repository", "", "repository ID")
	create.Flags().StringVar(&name, "name", "New plan", "plan name")
	create.Flags().StringVar(&objective, "objective", "", "feature objective")
	create.Flags().BoolVar(&resume, "resume", true, "resume latest draft")
	root.AddCommand(create)
	list := &cobra.Command{Use: "list", Short: "List plans", Args: cobra.NoArgs, RunE: withClient(func(cmd *cobra.Command, _ []string, c rpc.PlanServiceClient) error {
		r, e := c.ListPlans(cmd.Context(), connect.NewRequest(&pb.ListPlansRequest{RepositoryId: repo}))
		if e != nil {
			return e
		}
		return print(cmd, r.Msg)
	})}
	list.Flags().StringVar(&repo, "repository", "", "repository ID")
	root.AddCommand(list)
	for _, command := range []string{"show", "preview", "operations", "checklist", "prompt"} {
		var radius, revision uint32
		var claimID, claimText, claimKind string
		var targets []string
		var removeClaim bool
		child := &cobra.Command{Use: command + " <plan-name-or-id>", Short: command + " a plan", Args: cobra.ExactArgs(1), RunE: withClient(func(cmd *cobra.Command, args []string, c rpc.PlanServiceClient) error {
			request := connect.NewRequest(&pb.GetPlanRequest{PlanId: args[0]})
			if cmd.Flags().Changed("revision") {
				request.Msg.Revision = &revision
			}
			switch command {
			case "prompt":
				r, e := c.GetPrompt(cmd.Context(), request)
				if e != nil {
					return e
				}
				_, e = fmt.Fprint(cmd.OutOrStdout(), r.Msg.Text)
				return e
			case "operations":
				r, e := c.ListOperations(cmd.Context(), request)
				if e != nil {
					return e
				}
				return print(cmd, r.Msg)
			case "checklist":
				if cmd.Flags().Changed("text") || removeClaim {
					p, e := c.GetPlan(cmd.Context(), connect.NewRequest(&pb.GetPlanRequest{PlanId: args[0]}))
					if e != nil {
						return e
					}
					op := &pb.PlanOperation{Action: &pb.PlanOperation_Checklist{Checklist: &pb.PlanChecklistItem{Id: claimID, Text: claimText, VerificationKind: claimKind, ResourceKeys: targets}}}
					if removeClaim {
						if claimID == "" {
							return fmt.Errorf("--id is required with --remove")
						}
						op.Action = &pb.PlanOperation_RemoveChecklist{RemoveChecklist: &pb.PlanResourceKey{Key: claimID}}
					}
					_, e = c.AppendOperations(cmd.Context(), connect.NewRequest(&pb.AppendPlanOperationsRequest{PlanId: args[0], ExpectedRevision: p.Msg.Plan.Revision, RequestId: uuid.NewString(), Operations: []*pb.PlanOperation{op}}))
					if e != nil {
						return e
					}
				}
				r, e := c.GetChecklist(cmd.Context(), request)
				if e != nil {
					return e
				}
				return print(cmd, r.Msg)
			default:
				combined := &pb.PlanProjectionResponse{}
				cursor := ""
				for {
					r, e := c.GetProjection(cmd.Context(), connect.NewRequest(&pb.PlanProjectionRequest{PlanId: args[0], Revision: request.Msg.Revision, Radius: radius, Cursor: cursor}))
					if e != nil {
						return e
					}
					combined.Plan = r.Msg.Plan
					combined.Revision = r.Msg.Revision
					combined.WorkspaceHash = r.Msg.WorkspaceHash
					combined.Operations = r.Msg.Operations
					combined.Resources = append(combined.Resources, r.Msg.Resources...)
					cursor = r.Msg.NextCursor
					if cursor == "" {
						combined.Complete = true
						break
					}
				}
				return print(cmd, combined)
			}
		})}
		child.Flags().Uint32Var(&radius, "radius", 0, "connected workspace context hops")
		child.Flags().Uint32Var(&revision, "revision", 0, "read a specific persisted plan revision")
		if command == "checklist" {
			child.Flags().StringVar(&claimID, "id", "", "stable claim ID to edit or remove")
			child.Flags().StringVar(&claimText, "text", "", "add or replace requirement text")
			child.Flags().StringVar(&claimKind, "kind", "manual", "verification kind")
			child.Flags().StringSliceVar(&targets, "targets", nil, "resource keys for this claim")
			child.Flags().BoolVar(&removeClaim, "remove", false, "remove the claim identified by --id")
		}
		root.AddCommand(child)
	}
	for _, action := range []string{"include", "exclude"} {
		var role string
		child := &cobra.Command{Use: action + " <plan-name-or-id> <resource-name-or-key>", Short: action + " a resource", Args: cobra.ExactArgs(2), RunE: withClient(func(cmd *cobra.Command, args []string, c rpc.PlanServiceClient) error {
			p, e := c.GetPlan(cmd.Context(), connect.NewRequest(&pb.GetPlanRequest{PlanId: args[0]}))
			if e != nil {
				return e
			}
			op := &pb.PlanOperation{}
			if action == "include" {
				op.Action = &pb.PlanOperation_Include{Include: &pb.PlanMember{ResourceKey: args[1], Role: role, Explicit: true}}
			} else {
				op.Action = &pb.PlanOperation_Exclude{Exclude: &pb.PlanResourceKey{Key: args[1]}}
			}
			r, e := c.AppendOperations(cmd.Context(), connect.NewRequest(&pb.AppendPlanOperationsRequest{PlanId: args[0], ExpectedRevision: p.Msg.Plan.Revision, RequestId: uuid.NewString(), Operations: []*pb.PlanOperation{op}}))
			if e != nil {
				return e
			}
			return print(cmd, r.Msg)
		})}
		child.Flags().StringVar(&role, "role", "change", "change, preserve, or context")
		root.AddCommand(child)
	}
	for _, status := range []string{"ready", "draft", "archived"} {
		root.AddCommand(&cobra.Command{Use: status + " <plan-name-or-id>", Short: "Mark plan " + status, Args: cobra.ExactArgs(1), RunE: withClient(func(cmd *cobra.Command, args []string, c rpc.PlanServiceClient) error {
			p, e := c.GetPlan(cmd.Context(), connect.NewRequest(&pb.GetPlanRequest{PlanId: args[0]}))
			if e != nil {
				return e
			}
			r, e := c.UpdatePlan(cmd.Context(), connect.NewRequest(&pb.UpdatePlanRequest{PlanId: args[0], ExpectedRevision: p.Msg.Plan.Revision, Status: proto.String(status)}))
			if e != nil {
				return e
			}
			return print(cmd, r.Msg)
		})})
	}
	root.AddCommand(&cobra.Command{Use: "note <plan-name-or-id> <view-name-or-key>", Short: "Read a linked view note", Args: cobra.ExactArgs(2), RunE: withClient(func(cmd *cobra.Command, args []string, c rpc.PlanServiceClient) error {
		r, e := c.GetNote(cmd.Context(), connect.NewRequest(&pb.PlanNoteRequest{PlanId: args[0], ViewKey: args[1]}))
		if e != nil {
			return e
		}
		_, e = fmt.Fprint(cmd.OutOrStdout(), r.Msg.Note.Content)
		return e
	})})
	root.AddCommand(&cobra.Command{Use: "append <plan-name-or-id> <json-file|->", Short: "Append typed operations, including checklist and note edits", Args: cobra.ExactArgs(2), RunE: withClient(func(cmd *cobra.Command, args []string, c rpc.PlanServiceClient) error {
		var b []byte
		var err error
		if args[1] == "-" {
			b, err = io.ReadAll(cmd.InOrStdin())
		} else {
			b, err = os.ReadFile(args[1])
		}
		if err != nil {
			return err
		}
		req := &pb.AppendPlanOperationsRequest{}
		if err = protojson.Unmarshal(b, req); err != nil {
			return err
		}
		req.PlanId = args[0]
		if req.RequestId == "" {
			req.RequestId = uuid.NewString()
		}
		r, e := c.AppendOperations(cmd.Context(), connect.NewRequest(req))
		if e != nil {
			return e
		}
		return print(cmd, r.Msg)
	})})
	return root
}

func resolveResource(cmd *cobra.Command, c rpc.PlanServiceClient, id, selector string, viewOnly bool) (string, error) {
	request := &pb.PlanProjectionRequest{PlanId: id, FullWorkspace: true}
	graph := plans.Graph{}
	for {
		result, err := c.GetProjection(cmd.Context(), connect.NewRequest(request))
		if err != nil {
			return "", err
		}
		for _, item := range result.Msg.Resources {
			if item.Resource != nil {
				graph[item.Resource.Key] = item.Resource
			}
		}
		if result.Msg.NextCursor == "" {
			break
		}
		request.Cursor = result.Msg.NextCursor
		request.Revision = &result.Msg.Revision
	}
	if r := graph[selector]; r != nil && (!viewOnly || r.GetView() != nil) {
		return selector, nil
	}
	match := ""
	for key, r := range graph {
		if viewOnly && r.GetView() == nil {
			continue
		}
		if plans.Describe(graph, r) == selector {
			if match != "" {
				return "", fmt.Errorf("object name %q is ambiguous; use a resource key from tld plan show", selector)
			}
			match = key
		}
	}
	if match == "" {
		return "", fmt.Errorf("object %q not found in plan workspace", selector)
	}
	return match, nil
}
