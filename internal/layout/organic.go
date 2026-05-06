// Package layout provides force-directed graph layout algorithms for the local
// watch materializer. The organic layout is a port of backend-wrapper/pkg/layout/organic.go
// adapted for int64 element IDs used in the SQLite-backed watch store.
package layout

import (
	"math"
	"math/rand/v2"
	"time"

	"github.com/mertcikla/tld/internal/workspace"
)

const (
	NodeWidth     = 200.0
	NodeHeight    = 120.0
	Iterations    = 300
	AlphaStart    = 1.0
	AlphaMin      = 0.001
	VelocityDecay = 0.6
)

// Tunable layout parameters — override via environment variables.
var (
	layoutConfig    = workspace.ResolveWatchLayoutConfig()
	LinkDistance    = layoutConfig.LinkDistance
	ChargeStrength  = layoutConfig.ChargeStrength
	CollideRadius   = layoutConfig.CollideRadius
	GravityStrength = layoutConfig.GravityStrength
)

// Node is a positioned graph node. ID matches an element_id in the placements table.
type Node struct {
	ID     int64
	X, Y   float64
	VX, VY float64
	Degree int // number of connected edges
}

// Edge is a directed connection between two Nodes.
type Edge struct {
	Source *Node
	Target *Node
}

// OrganicLayout applies a D3-like force-directed layout to nodes and edges,
// mutating node X/Y positions in place.
func OrganicLayout(nodes []*Node, edges []*Edge) {
	if len(nodes) == 0 {
		return
	}

	// Initialize degrees.
	for _, e := range edges {
		if e.Source != nil && e.Target != nil {
			e.Source.Degree++
			e.Target.Degree++
		}
	}

	// Initialize random generator and scatter unpositioned nodes to avoid exact overlapping.
	// #nosec G404
	rng := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0))
	for _, n := range nodes {
		// D3 initialises unpositioned nodes in a phyllotaxis arrangement; we
		// scatter in a 2:1 aspect ratio to match the canvas.
		if n.X == 0 && n.Y == 0 {
			n.X = (rng.Float64() - 0.5) * float64(len(nodes)) * 10.0
			n.Y = (rng.Float64() - 0.5) * float64(len(nodes)) * 5.0
		}
	}

	alphaDecay := 1.0 - math.Pow(AlphaMin, 1.0/float64(Iterations))
	alpha := AlphaStart

	for range Iterations {
		alpha += (AlphaMin - alpha) * alphaDecay

		// 0. Gravity — pull nodes toward the centre.
		gravityStrength := GravityStrength * alpha
		for _, n := range nodes {
			n.VX -= n.X * gravityStrength
			n.VY -= n.Y * gravityStrength * 2.0 // 2x in Y nudges layout toward 2:1 aspect ratio
		}

		// 1. Many-Body Force (repulsion, O(n²)).
		for i := range nodes {
			n1 := nodes[i]
			for j := i + 1; j < len(nodes); j++ {
				n2 := nodes[j]

				dx := n2.X - n1.X
				dy := n2.Y - n1.Y
				distSq := dx*dx + dy*dy
				if distSq == 0 {
					dx = (rng.Float64() - 0.5) * 1e-3
					dy = (rng.Float64() - 0.5) * 1e-3
					distSq = dx*dx + dy*dy
				}
				dist := math.Sqrt(distSq)
				if dist < 1.0 {
					distSq = 1.0
					dist = 1.0
				}

				w := ChargeStrength * alpha / (distSq * dist)
				fvX := dx * w
				fvY := dy * w

				n2.VX += fvX
				n2.VY += fvY
				n1.VX -= fvX
				n1.VY -= fvY
			}
		}

		// 2. Link Force (spring attraction along edges).
		for _, e := range edges {
			s := e.Source
			t := e.Target
			if s == nil || t == nil {
				continue
			}

			dx := t.X + t.VX - (s.X + s.VX)
			dy := t.Y + t.VY - (s.Y + s.VY)
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist == 0 {
				dx = (rng.Float64() - 0.5) * 1e-3
				dy = (rng.Float64() - 0.5) * 1e-3
				dist = math.Sqrt(dx*dx + dy*dy)
			}

			diff := (dist - LinkDistance) / dist * alpha
			biasS := float64(s.Degree) / float64(s.Degree+t.Degree)
			if s.Degree+t.Degree == 0 {
				biasS = 0.5
			}
			biasT := 1.0 - biasS

			t.VX -= dx * diff * biasS
			t.VY -= dy * diff * biasS
			s.VX += dx * diff * biasT
			s.VY += dy * diff * biasT
		}

		// 3. Collision Force (simple O(n²) — fine for <1 000 nodes).
		r := CollideRadius * 2.0
		rSq := r * r
		for i := range nodes {
			n1 := nodes[i]
			for j := i + 1; j < len(nodes); j++ {
				n2 := nodes[j]

				dx := n2.X + n2.VX - (n1.X + n1.VX)
				dy := n2.Y + n2.VY - (n1.Y + n1.VY)
				distSq := dx*dx + dy*dy

				if distSq < rSq {
					dist := math.Sqrt(distSq)
					if dist == 0 {
						dx = (rng.Float64() - 0.5) * 1e-3
						dy = (rng.Float64() - 0.5) * 1e-3
						dist = math.Sqrt(dx*dx + dy*dy)
					}
					diff := (r - dist) / dist * 0.7

					n2.VX += dx * diff * 0.5
					n2.VY += dy * diff * 0.5
					n1.VX -= dx * diff * 0.5
					n1.VY -= dy * diff * 0.5
				}
			}
		}

		// 4. Velocity verlet integration.
		for _, n := range nodes {
			n.VX *= VelocityDecay
			n.VY *= VelocityDecay
			n.X += n.VX
			n.Y += n.VY
		}
	}

	// 5. Shift centroid to origin.
	var sumX, sumY float64
	for _, n := range nodes {
		sumX += n.X
		sumY += n.Y
	}
	if len(nodes) > 0 {
		cx := sumX / float64(len(nodes))
		cy := sumY / float64(len(nodes))
		for _, n := range nodes {
			n.X -= cx
			n.Y -= cy
		}
	}

	// 6. Apply the same top-left offset as the frontend `runForce`:
	//    positions.set(n.id, { x: n.x - NODE_W / 2, y: n.y - NODE_H / 2 })
	for _, n := range nodes {
		n.X -= NodeWidth / 2.0
		n.Y -= NodeHeight / 2.0
	}
}
