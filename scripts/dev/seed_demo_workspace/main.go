package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mertcikla/tld/v2/internal/workspace"
)

type seedElement struct {
	ID          int
	Name        string
	Kind        string
	Description string
	Technology  string
	LogoURL     string
	Language    string
	FilePath    string
	Symbol      string
	Tags        []string
	HasView     bool
	ViewLabel   string
}

type seedPlacement struct {
	ViewID    int
	ElementID int
	X         float64
	Y         float64
}

type seedConnector struct {
	ViewID       int
	SourceID     int
	TargetID     int
	Label        string
	Direction    string
	Style        string
	SourceHandle string
	TargetHandle string
}

type seedView struct {
	ID          int
	Ref         string
	Name        string
	Description string
	LevelLabel  string
}

var nonRefChars = regexp.MustCompile(`[^a-z0-9._-]+`)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "seed demo workspace: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	outDir := flag.String("out", filepath.Join(os.TempDir(), "tld-demo-workspace"), "workspace directory to write")
	force := flag.Bool("force", false, "replace existing workspace files")
	flag.Parse()

	target := strings.TrimSpace(*outDir)
	if target == "" {
		target = "."
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if !*force {
		for _, name := range []string{".tld.yaml", "elements.yaml", "connectors.yaml"} {
			if _, err := os.Stat(filepath.Join(target, name)); err == nil {
				return fmt.Errorf("%s already exists; pass -force to replace it", filepath.Join(target, name))
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("check %s: %w", name, err)
			}
		}
	}

	refs := elementRefs()
	ws := &workspace.Workspace{
		Dir: target,
		WorkspaceConfig: &workspace.WorkspaceConfig{
			ProjectName: "Demo Commerce Architecture",
		},
		Elements:   make(map[string]*workspace.Element),
		Connectors: make(map[string]*workspace.Connector),
	}

	ws.Elements[views[1].Ref] = &workspace.Element{
		Name:        views[1].Name,
		Kind:        "workspace",
		Description: views[1].Description,
		Technology:  "woocommerce",
		LogoURL:     iconURL("woocommerce"),
		HasView:     true,
		ViewLabel:   views[1].LevelLabel,
		Placements: []workspace.ViewPlacement{{
			ParentRef: "root",
		}},
	}

	for _, seed := range elements {
		ref := refs[seed.ID]
		ws.Elements[ref] = &workspace.Element{
			Name:        seed.Name,
			Kind:        seed.Kind,
			Description: seed.Description,
			Technology:  seed.Technology,
			LogoURL:     seed.LogoURL,
			Language:    seed.Language,
			FilePath:    seed.FilePath,
			Symbol:      seed.Symbol,
			Tags:        seed.Tags,
			HasView:     seed.HasView,
			ViewLabel:   seed.ViewLabel,
			Placements:  placementsFor(seed.ID, refs),
		}
	}

	for _, seed := range connectors {
		connector := &workspace.Connector{
			View:         views[seed.ViewID].Ref,
			Source:       refs[seed.SourceID],
			Target:       refs[seed.TargetID],
			Label:        seed.Label,
			Direction:    seed.Direction,
			Style:        seed.Style,
			SourceHandle: seed.SourceHandle,
			TargetHandle: seed.TargetHandle,
		}
		ws.Connectors[workspace.ConnectorKey(connector)] = connector
	}

	if err := workspace.Save(ws); err != nil {
		return err
	}
	if err := writeWorkspaceConfig(target); err != nil {
		return err
	}

	fmt.Printf("Wrote demo workspace to %s\n", target)
	fmt.Printf("Elements: %d\n", len(ws.Elements))
	fmt.Printf("Connectors: %d\n", len(ws.Connectors))
	return nil
}

func placementsFor(elementID int, refs map[int]string) []workspace.ViewPlacement {
	var out []workspace.ViewPlacement
	for _, placement := range placements {
		if placement.ElementID != elementID {
			continue
		}
		out = append(out, workspace.ViewPlacement{
			ParentRef:    views[placement.ViewID].Ref,
			PositionX:    placement.X,
			PositionY:    placement.Y,
			PositionXSet: true,
			PositionYSet: true,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func elementRefs() map[int]string {
	refs := make(map[int]string, len(elements))
	used := map[string]int{"root": 1}
	for _, seed := range elements {
		ref := slugify(seed.Name)
		if ref == "" {
			ref = fmt.Sprintf("element-%d", seed.ID)
		}
		if count := used[ref]; count > 0 {
			used[ref] = count + 1
			ref = fmt.Sprintf("%s-%d", ref, count+1)
		} else {
			used[ref] = 1
		}
		refs[seed.ID] = ref
	}
	return refs
}

func slugify(value string) string {
	slug := strings.ToLower(strings.TrimSpace(value))
	slug = nonRefChars.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-._")
	return slug
}

func writeWorkspaceConfig(dir string) error {
	body := []byte("project_name: Demo Commerce Architecture\n")
	if err := os.WriteFile(filepath.Join(dir, ".tld.yaml"), body, 0o600); err != nil {
		return fmt.Errorf("write .tld.yaml: %w", err)
	}
	return nil
}

func iconURL(slug string) string {
	return "/icons/" + slug + ".svg"
}

var views = map[int]seedView{
	1: {ID: 1, Ref: "system-context", Name: "e-Commerce", Description: "Top-level system context view", LevelLabel: "Context"},
	2: {ID: 2, Ref: "web-app", Name: "Web App - Containers", Description: "Container-level view of the Web App", LevelLabel: "Container"},
	3: {ID: 3, Ref: "api-gateway", Name: "API Gateway - Internals", Description: "Request flow through gateway policies, handlers, and integrations", LevelLabel: "Component"},
	4: {ID: 4, Ref: "checkout-flow", Name: "Checkout Flow - Components", Description: "Component-level checkout flow with steps, state, validation, and submission", LevelLabel: "Component"},
	5: {ID: 5, Ref: "checkout-orchestrator", Name: "Checkout Orchestrator - Modules", Description: "Module-level checkout orchestration and backend handoff details", LevelLabel: "Module"},
	6: {ID: 6, Ref: "submit-order-action", Name: "Submit Order Action - Code Path", Description: "Code-level path for final checkout submission", LevelLabel: "Code"},
}

var elements = []seedElement{
	{ID: 1, Name: "User", Kind: "person", Description: "End user of the system", Technology: "User", LogoURL: iconURL("user"), Tags: []string{"external"}},
	{ID: 2, Name: "Web App", Kind: "service", Description: "React single-page application", Technology: "React", LogoURL: iconURL("react"), Tags: []string{"frontend"}, HasView: true, ViewLabel: "Container"},
	{ID: 3, Name: "API Gateway", Kind: "service", Description: "REST API gateway", Technology: "Go", LogoURL: iconURL("go"), Tags: []string{"backend"}, HasView: true, ViewLabel: "Component"},
	{ID: 4, Name: "Auth Service", Kind: "service", Description: "Handles authentication & sessions", Technology: "Go", LogoURL: iconURL("go"), Tags: []string{"backend"}},
	{ID: 9, Name: "CDN", Kind: "service", Description: "Content delivery network", Technology: "Cloudflare", LogoURL: iconURL("cloudflare"), Tags: []string{"external", "infrastructure"}},
	{ID: 5, Name: "App Shell", Kind: "component", Description: "Top-level React layout, providers, and navigation chrome", Technology: "React", LogoURL: iconURL("react"), Tags: []string{"frontend", "shell"}},
	{ID: 6, Name: "Route Map", Kind: "component", Description: "Client-side route definitions and page loading boundaries", Technology: "Reactrouter", LogoURL: iconURL("reactrouter"), Tags: []string{"frontend", "routing"}},
	{ID: 7, Name: "Design System", Kind: "component", Description: "Shared components, tokens, and responsive primitives", Technology: "Tailwind CSS", LogoURL: iconURL("tailwindcss"), Tags: []string{"frontend", "design-system"}},
	{ID: 8, Name: "Product Catalog", Kind: "component", Description: "Product grids, cards, recommendations, and detail panels", Technology: "TypeScript", LogoURL: iconURL("typescript"), Tags: []string{"frontend", "catalog"}},
	{ID: 11, Name: "Cart State", Kind: "component", Description: "Local cart model, optimistic updates, and persistence hooks", Technology: "Zustand", LogoURL: iconURL("zustand"), Tags: []string{"frontend", "state"}},
	{ID: 12, Name: "Checkout Flow", Kind: "component", Description: "Multi-step checkout experience for shipping, taxes, and payment", Technology: "React", LogoURL: iconURL("react"), Tags: []string{"frontend", "checkout"}, HasView: true, ViewLabel: "Component"},
	{ID: 13, Name: "API Client", Kind: "api", Description: "Typed fetch layer for backend calls, retries, and response mapping", Technology: "TypeScript", LogoURL: iconURL("typescript"), Tags: []string{"frontend", "api"}},
	{ID: 14, Name: "Auth Adapter", Kind: "component", Description: "Session state, guards, and auth provider integration", Technology: "Clerk", LogoURL: iconURL("clerk"), Tags: []string{"frontend", "auth"}},
	{ID: 15, Name: "Payment Adapter", Kind: "component", Description: "Payment intent orchestration and checkout handoff", Technology: "TypeScript", LogoURL: iconURL("typescript"), Tags: []string{"frontend", "payments"}},
	{ID: 16, Name: "Analytics Client", Kind: "service", Description: "Product events, funnels, and release health telemetry", Technology: "Datadog", LogoURL: iconURL("datadog"), Tags: []string{"observability"}},
	{ID: 17, Name: "Feature Flags", Kind: "service", Description: "Progressive rollout and experiment targeting for UI features", Technology: "Cloudflare", LogoURL: iconURL("cloudflare"), Tags: []string{"frontend", "edge"}},
	{ID: 18, Name: "State Store", Kind: "component", Description: "Shared client state for user, catalog, checkout, and preferences", Technology: "Zustand", LogoURL: iconURL("zustand"), Tags: []string{"frontend", "state"}},
	{ID: 19, Name: "Form Validation", Kind: "component", Description: "Typed validation schemas for account and checkout forms", Technology: "Zod", LogoURL: iconURL("zod"), Tags: []string{"frontend", "checkout"}},
	{ID: 20, Name: "Error Boundary", Kind: "component", Description: "Crash recovery surfaces and structured error reporting", Technology: "Sentry", LogoURL: iconURL("sentry"), Tags: []string{"frontend", "observability"}},
	{ID: 21, Name: "Asset Pipeline", Kind: "service", Description: "Vite build graph, chunking, prefetching, and static assets", Technology: "Vite", LogoURL: iconURL("vite"), Tags: []string{"frontend", "build"}},
	{ID: 22, Name: "Storybook", Kind: "service", Description: "Component documentation and visual review workflows", Technology: "Storybook", LogoURL: iconURL("storybook"), Tags: []string{"design-system", "testing"}},
	{ID: 23, Name: "E2E Tests", Kind: "service", Description: "Critical path browser automation for catalog and checkout flows", Technology: "Playwright", LogoURL: iconURL("playwright"), Tags: []string{"testing"}},
	{ID: 24, Name: "Edge Router", Kind: "api", Description: "Ingress routing for public REST and webhook traffic", Technology: "Nginx", LogoURL: iconURL("nginx"), Tags: []string{"backend", "gateway", "traffic"}},
	{ID: 25, Name: "Rate Limiter", Kind: "service", Description: "Per-user and per-IP quotas before requests reach handlers", Technology: "Redis", LogoURL: iconURL("redis"), Tags: []string{"backend", "policy", "traffic"}},
	{ID: 26, Name: "Auth Middleware", Kind: "service", Description: "Session, API key, and bearer token checks for protected routes", Technology: "Go", LogoURL: iconURL("go"), Tags: []string{"backend", "auth", "security"}},
	{ID: 27, Name: "Request Validator", Kind: "component", Description: "Schema validation and request normalization before dispatch", Technology: "Go", LogoURL: iconURL("go"), Tags: []string{"backend", "policy"}},
	{ID: 28, Name: "REST Controllers", Kind: "api", Description: "Route handlers that translate HTTP requests into domain calls", Technology: "Go", LogoURL: iconURL("go"), Tags: []string{"backend", "gateway"}},
	{ID: 29, Name: "Service Client", Kind: "service", Description: "Internal client for backend services and persistence boundaries", Technology: "Go", LogoURL: iconURL("go"), Tags: []string{"backend", "integration"}},
	{ID: 30, Name: "Response Cache", Kind: "database", Description: "Short-lived cache for read-heavy API responses", Technology: "Redis", LogoURL: iconURL("redis"), Tags: []string{"backend", "data", "cache"}},
	{ID: 31, Name: "OpenAPI Contract", Kind: "component", Description: "Public API contract used by clients, docs, and request checks", Technology: "OpenAPI", LogoURL: iconURL("openapi"), Tags: []string{"backend", "api", "contract"}},
	{ID: 32, Name: "Checkout Route Shell", Kind: "component", Description: "Route boundary that loads checkout data and preserves deep links", Technology: "Reactrouter", LogoURL: iconURL("reactrouter"), Tags: []string{"frontend", "checkout", "routing"}},
	{ID: 33, Name: "Checkout Stepper", Kind: "component", Description: "Step controller for shipping, delivery, payment, and review screens", Technology: "React", LogoURL: iconURL("react"), Tags: []string{"frontend", "checkout", "ui"}},
	{ID: 34, Name: "Checkout Store", Kind: "component", Description: "Draft checkout state, selected step, and pending submission flags", Technology: "Zustand", LogoURL: iconURL("zustand"), Tags: []string{"frontend", "checkout", "state"}},
	{ID: 35, Name: "Session Gate", Kind: "component", Description: "Requires a signed-in user before payment or order submission", Technology: "Clerk", LogoURL: iconURL("clerk"), Tags: []string{"frontend", "checkout", "auth"}},
	{ID: 36, Name: "Shipping Form", Kind: "component", Description: "Address collection with field-level errors and saved profile defaults", Technology: "React", LogoURL: iconURL("react"), Tags: []string{"frontend", "checkout", "form"}},
	{ID: 37, Name: "Delivery Form", Kind: "component", Description: "Shipping option selection, ETA display, and price recalculation", Technology: "React", LogoURL: iconURL("react"), Tags: []string{"frontend", "checkout", "form"}},
	{ID: 38, Name: "Payment Form", Kind: "component", Description: "Payment method entry and client-side payment readiness checks", Technology: "React", LogoURL: iconURL("react"), Tags: []string{"frontend", "checkout", "payments"}},
	{ID: 39, Name: "Checkout Orchestrator", Kind: "component", Description: "Coordinates validated checkout data, payment intent setup, and order submission", Technology: "TypeScript", LogoURL: iconURL("typescript"), Tags: []string{"frontend", "checkout", "orchestration"}, HasView: true, ViewLabel: "Module"},
	{ID: 40, Name: "Checkout API Mutation", Kind: "api", Description: "React Query mutation for placing the order through the typed API client", Technology: "React Query", LogoURL: iconURL("react-query"), Tags: []string{"frontend", "checkout", "api"}},
	{ID: 41, Name: "Checkout Telemetry", Kind: "component", Description: "Checkout funnel, validation, and payment health event reporting", Technology: "Datadog", LogoURL: iconURL("datadog"), Tags: []string{"frontend", "checkout", "observability"}},
	{ID: 42, Name: "checkoutMachine.ts", Kind: "module", Description: "Transition rules for advancing, blocking, or rewinding checkout steps", Technology: "TypeScript", LogoURL: iconURL("typescript"), Language: "typescript", FilePath: "frontend/src/features/checkout/checkoutMachine.ts", Tags: []string{"frontend", "checkout", "module"}},
	{ID: 43, Name: "checkoutSchema.ts", Kind: "module", Description: "Zod schema for shipping, delivery, payment, and consent fields", Technology: "Zod", LogoURL: iconURL("zod"), Language: "typescript", FilePath: "frontend/src/features/checkout/checkoutSchema.ts", Tags: []string{"frontend", "checkout", "validation"}},
	{ID: 44, Name: "buildOrderPayload.ts", Kind: "module", Description: "Maps checkout state into the backend order request contract", Technology: "TypeScript", LogoURL: iconURL("typescript"), Language: "typescript", FilePath: "frontend/src/features/checkout/buildOrderPayload.ts", Tags: []string{"frontend", "checkout", "module"}},
	{ID: 45, Name: "Submit Order Action", Kind: "module", Description: "Final orchestration module for payment intent creation and order persistence", Technology: "TypeScript", LogoURL: iconURL("typescript"), Language: "typescript", FilePath: "frontend/src/features/checkout/submitOrder.ts", Tags: []string{"frontend", "checkout", "code"}, HasView: true, ViewLabel: "Code"},
	{ID: 46, Name: "paymentIntentClient.ts", Kind: "module", Description: "Typed client for creating, refreshing, and confirming payment intents", Technology: "TypeScript", LogoURL: iconURL("typescript"), Language: "typescript", FilePath: "frontend/src/features/checkout/paymentIntentClient.ts", Tags: []string{"frontend", "checkout", "payments"}},
	{ID: 47, Name: "idempotencyKey.ts", Kind: "module", Description: "Generates stable order submission keys across retries and page reloads", Technology: "TypeScript", LogoURL: iconURL("typescript"), Language: "typescript", FilePath: "frontend/src/features/checkout/idempotencyKey.ts", Tags: []string{"frontend", "checkout", "resilience"}},
	{ID: 48, Name: "useCheckoutMutation.ts", Kind: "module", Description: "Mutation hook for placing orders and invalidating checkout cache", Technology: "React Query", LogoURL: iconURL("react-query"), Language: "typescript", FilePath: "frontend/src/features/checkout/useCheckoutMutation.ts", Tags: []string{"frontend", "checkout", "api"}},
	{ID: 49, Name: "checkoutErrorMapper.ts", Kind: "module", Description: "Maps payment, validation, and API failures into recoverable UI states", Technology: "Sentry", LogoURL: iconURL("sentry"), Language: "typescript", FilePath: "frontend/src/features/checkout/checkoutErrorMapper.ts", Tags: []string{"frontend", "checkout", "observability"}},
	{ID: 50, Name: "handleSubmit()", Kind: "function", Description: "Entry point called by the review screen submit button", Technology: "TypeScript", LogoURL: iconURL("typescript"), Language: "typescript", FilePath: "frontend/src/features/checkout/submitOrder.ts", Symbol: "handleSubmit", Tags: []string{"frontend", "checkout", "code"}},
	{ID: 51, Name: "readCheckoutState()", Kind: "function", Description: "Reads cart, user, delivery, and payment drafts from the checkout store", Technology: "Zustand", LogoURL: iconURL("zustand"), Language: "typescript", FilePath: "frontend/src/features/checkout/checkoutStore.ts", Symbol: "readCheckoutState", Tags: []string{"frontend", "checkout", "state"}},
	{ID: 52, Name: "validateCheckoutInput()", Kind: "function", Description: "Parses draft checkout input through the final submission schema", Technology: "Zod", LogoURL: iconURL("zod"), Language: "typescript", FilePath: "frontend/src/features/checkout/checkoutSchema.ts", Symbol: "validateCheckoutInput", Tags: []string{"frontend", "checkout", "validation"}},
	{ID: 53, Name: "buildOrderPayload()", Kind: "function", Description: "Builds the order request body with cart lines, totals, and fulfillment data", Technology: "TypeScript", LogoURL: iconURL("typescript"), Language: "typescript", FilePath: "frontend/src/features/checkout/buildOrderPayload.ts", Symbol: "buildOrderPayload", Tags: []string{"frontend", "checkout", "api"}},
	{ID: 54, Name: "ensurePaymentIntent()", Kind: "function", Description: "Creates or refreshes the payment intent before persisting the order", Technology: "TypeScript", LogoURL: iconURL("typescript"), Language: "typescript", FilePath: "frontend/src/features/checkout/paymentIntentClient.ts", Symbol: "ensurePaymentIntent", Tags: []string{"frontend", "checkout", "payments"}},
	{ID: 55, Name: "persistOrder()", Kind: "function", Description: "Commits the validated order payload through the checkout mutation", Technology: "React Query", LogoURL: iconURL("react-query"), Language: "typescript", FilePath: "frontend/src/features/checkout/useCheckoutMutation.ts", Symbol: "persistOrder", Tags: []string{"frontend", "checkout", "api"}},
	{ID: 56, Name: "clearCartAndRedirect()", Kind: "function", Description: "Clears local cart state and navigates to the order confirmation route", Technology: "Reactrouter", LogoURL: iconURL("reactrouter"), Language: "typescript", FilePath: "frontend/src/features/checkout/submitOrder.ts", Symbol: "clearCartAndRedirect", Tags: []string{"frontend", "checkout", "routing"}},
	{ID: 57, Name: "captureCheckoutFailure()", Kind: "function", Description: "Captures structured submission failures with payment and step context", Technology: "Sentry", LogoURL: iconURL("sentry"), Language: "typescript", FilePath: "frontend/src/features/checkout/checkoutErrorMapper.ts", Symbol: "captureCheckoutFailure", Tags: []string{"frontend", "checkout", "observability"}},
}

var placements = []seedPlacement{
	{ViewID: 1, ElementID: 1, X: 80, Y: 200},
	{ViewID: 1, ElementID: 2, X: 380, Y: 200},
	{ViewID: 1, ElementID: 3, X: 680, Y: 200},
	{ViewID: 1, ElementID: 4, X: 380, Y: 0},
	{ViewID: 1, ElementID: 9, X: 380, Y: 400},
	{ViewID: 2, ElementID: 5, X: 290, Y: 150},
	{ViewID: 2, ElementID: 7, X: 290, Y: 360},
	{ViewID: 2, ElementID: 6, X: 540, Y: 60},
	{ViewID: 2, ElementID: 18, X: 540, Y: 270},
	{ViewID: 2, ElementID: 20, X: 540, Y: 480},
	{ViewID: 2, ElementID: 22, X: 290, Y: 560},
	{ViewID: 2, ElementID: 8, X: 790, Y: 60},
	{ViewID: 2, ElementID: 11, X: 790, Y: 480},
	{ViewID: 2, ElementID: 13, X: 1040, Y: 110},
	{ViewID: 2, ElementID: 12, X: 1040, Y: 350},
	{ViewID: 2, ElementID: 19, X: 1040, Y: 560},
	{ViewID: 2, ElementID: 14, X: 1290, Y: 60},
	{ViewID: 2, ElementID: 15, X: 1290, Y: 270},
	{ViewID: 2, ElementID: 23, X: 1290, Y: 560},
	{ViewID: 3, ElementID: 24, X: 80, Y: 180},
	{ViewID: 3, ElementID: 25, X: 330, Y: 80},
	{ViewID: 3, ElementID: 26, X: 330, Y: 280},
	{ViewID: 3, ElementID: 27, X: 580, Y: 180},
	{ViewID: 3, ElementID: 31, X: 670, Y: 30},
	{ViewID: 3, ElementID: 28, X: 830, Y: 180},
	{ViewID: 3, ElementID: 29, X: 1080, Y: 180},
	{ViewID: 3, ElementID: 30, X: 1080, Y: 380},
	{ViewID: 4, ElementID: 32, X: 80, Y: 160},
	{ViewID: 4, ElementID: 35, X: 330, Y: 60},
	{ViewID: 4, ElementID: 33, X: 330, Y: 260},
	{ViewID: 4, ElementID: 34, X: 580, Y: 60},
	{ViewID: 4, ElementID: 36, X: 580, Y: 260},
	{ViewID: 4, ElementID: 37, X: 830, Y: 260},
	{ViewID: 4, ElementID: 38, X: 1080, Y: 260},
	{ViewID: 4, ElementID: 39, X: 1330, Y: 260},
	{ViewID: 4, ElementID: 40, X: 1580, Y: 160},
	{ViewID: 4, ElementID: 41, X: 1580, Y: 380},
	{ViewID: 5, ElementID: 42, X: 80, Y: 180},
	{ViewID: 5, ElementID: 43, X: 330, Y: 60},
	{ViewID: 5, ElementID: 44, X: 330, Y: 300},
	{ViewID: 5, ElementID: 47, X: 580, Y: 60},
	{ViewID: 5, ElementID: 45, X: 580, Y: 300},
	{ViewID: 5, ElementID: 46, X: 830, Y: 180},
	{ViewID: 5, ElementID: 48, X: 1080, Y: 300},
	{ViewID: 5, ElementID: 49, X: 1080, Y: 60},
	{ViewID: 6, ElementID: 50, X: 80, Y: 220},
	{ViewID: 6, ElementID: 51, X: 330, Y: 80},
	{ViewID: 6, ElementID: 52, X: 330, Y: 360},
	{ViewID: 6, ElementID: 53, X: 580, Y: 220},
	{ViewID: 6, ElementID: 54, X: 830, Y: 80},
	{ViewID: 6, ElementID: 55, X: 1080, Y: 220},
	{ViewID: 6, ElementID: 56, X: 1330, Y: 220},
	{ViewID: 6, ElementID: 57, X: 1080, Y: 420},
}

var connectors = []seedConnector{
	{ViewID: 1, SourceID: 1, TargetID: 2, Label: "Uses", Direction: "forward", Style: "bezier", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 1, SourceID: 2, TargetID: 3, Label: "API calls", Direction: "forward", Style: "bezier", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 1, SourceID: 2, TargetID: 4, Label: "Auth", Direction: "forward", Style: "bezier", SourceHandle: "top", TargetHandle: "bottom"},
	{ViewID: 1, SourceID: 3, TargetID: 4, Direction: "both", Style: "bezier", SourceHandle: "top", TargetHandle: "right"},
	{ViewID: 1, SourceID: 2, TargetID: 9, Label: "Serves via", Direction: "backward", Style: "bezier", SourceHandle: "bottom", TargetHandle: "top"},
	{ViewID: 2, SourceID: 7, TargetID: 22, Direction: "forward", Style: "bezier", SourceHandle: "bottom", TargetHandle: "top"},
	{ViewID: 2, SourceID: 5, TargetID: 6, Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 2, SourceID: 5, TargetID: 18, Label: "Provides state", Direction: "forward", Style: "bezier", SourceHandle: "right", TargetHandle: "top"},
	{ViewID: 2, SourceID: 5, TargetID: 20, Direction: "forward", Style: "bezier", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 2, SourceID: 6, TargetID: 8, Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 2, SourceID: 18, TargetID: 11, Label: "Hydrates cart", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 2, SourceID: 8, TargetID: 11, Label: "Adds item", Direction: "forward", Style: "smoothstep", SourceHandle: "bottom", TargetHandle: "top"},
	{ViewID: 2, SourceID: 8, TargetID: 13, Label: "Loads products", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 2, SourceID: 11, TargetID: 12, Label: "Starts checkout", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 2, SourceID: 12, TargetID: 19, Label: "Validates steps", Direction: "forward", Style: "smoothstep", SourceHandle: "bottom", TargetHandle: "top"},
	{ViewID: 2, SourceID: 12, TargetID: 13, Label: "Submits order", Direction: "forward", Style: "smoothstep", SourceHandle: "top", TargetHandle: "bottom"},
	{ViewID: 2, SourceID: 12, TargetID: 14, Label: "Requires session", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 2, SourceID: 12, TargetID: 15, Label: "Payment handoff", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 2, SourceID: 13, TargetID: 14, Label: "Attaches token", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 2, SourceID: 12, TargetID: 23, Label: "Tests checkout", Direction: "forward", Style: "smoothstep", SourceHandle: "bottom", TargetHandle: "top"},
	{ViewID: 3, SourceID: 24, TargetID: 25, Label: "Checks quota", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 3, SourceID: 24, TargetID: 26, Label: "Authenticates", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 3, SourceID: 25, TargetID: 27, Label: "Allowed traffic", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 3, SourceID: 26, TargetID: 27, Label: "Authorized request", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 3, SourceID: 31, TargetID: 27, Label: "Defines schema", Direction: "forward", Style: "smoothstep", SourceHandle: "bottom", TargetHandle: "top"},
	{ViewID: 3, SourceID: 27, TargetID: 28, Label: "Dispatches", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 3, SourceID: 28, TargetID: 29, Label: "Calls domain", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 3, SourceID: 28, TargetID: 30, Label: "Reads cache", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 3, SourceID: 29, TargetID: 30, Label: "Stores response", Direction: "forward", Style: "smoothstep", SourceHandle: "bottom", TargetHandle: "top"},
	{ViewID: 3, SourceID: 30, TargetID: 24, Label: "Cached response", Direction: "backward", Style: "smoothstep", SourceHandle: "left", TargetHandle: "bottom"},
	{ViewID: 4, SourceID: 32, TargetID: 35, Label: "Checks session", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 4, SourceID: 32, TargetID: 33, Label: "Renders flow", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 4, SourceID: 35, TargetID: 34, Label: "Provides user", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 4, SourceID: 33, TargetID: 34, Label: "Reads step state", Direction: "forward", Style: "smoothstep", SourceHandle: "top", TargetHandle: "bottom"},
	{ViewID: 4, SourceID: 33, TargetID: 36, Label: "Collects shipping", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 4, SourceID: 36, TargetID: 37, Label: "Feeds delivery", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 4, SourceID: 37, TargetID: 38, Label: "Quotes payment", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 4, SourceID: 34, TargetID: 39, Label: "Coordinates draft", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "top"},
	{ViewID: 4, SourceID: 38, TargetID: 39, Label: "Payment input", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 4, SourceID: 39, TargetID: 40, Label: "Submits order", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 4, SourceID: 39, TargetID: 41, Label: "Emits funnel events", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 5, SourceID: 42, TargetID: 43, Label: "Validates guards", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 5, SourceID: 42, TargetID: 44, Label: "Builds payload", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 5, SourceID: 43, TargetID: 45, Label: "Blocks invalid input", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "top"},
	{ViewID: 5, SourceID: 44, TargetID: 45, Label: "Order payload", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 5, SourceID: 47, TargetID: 45, Label: "Adds retry key", Direction: "forward", Style: "smoothstep", SourceHandle: "bottom", TargetHandle: "top"},
	{ViewID: 5, SourceID: 45, TargetID: 46, Label: "Creates intent", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 5, SourceID: 45, TargetID: 48, Label: "Commits order", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 5, SourceID: 46, TargetID: 49, Label: "Payment failure", Direction: "forward", Style: "smoothstep", SourceHandle: "top", TargetHandle: "left"},
	{ViewID: 5, SourceID: 48, TargetID: 49, Label: "API failure", Direction: "forward", Style: "smoothstep", SourceHandle: "top", TargetHandle: "bottom"},
	{ViewID: 6, SourceID: 50, TargetID: 51, Label: "Reads cart and steps", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 6, SourceID: 51, TargetID: 52, Label: "Normalizes input", Direction: "forward", Style: "smoothstep", SourceHandle: "bottom", TargetHandle: "top"},
	{ViewID: 6, SourceID: 52, TargetID: 53, Label: "Validated data", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 6, SourceID: 53, TargetID: 54, Label: "Payment request", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 6, SourceID: 54, TargetID: 55, Label: "Intent ready", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 6, SourceID: 55, TargetID: 56, Label: "Success path", Direction: "forward", Style: "smoothstep", SourceHandle: "right", TargetHandle: "left"},
	{ViewID: 6, SourceID: 50, TargetID: 57, Label: "Catch boundary", Direction: "forward", Style: "smoothstep", SourceHandle: "bottom", TargetHandle: "left"},
	{ViewID: 6, SourceID: 54, TargetID: 57, Label: "Payment error", Direction: "forward", Style: "smoothstep", SourceHandle: "bottom", TargetHandle: "left"},
	{ViewID: 6, SourceID: 55, TargetID: 57, Label: "Mutation error", Direction: "forward", Style: "smoothstep", SourceHandle: "bottom", TargetHandle: "top"},
}
