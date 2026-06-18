package mermaid

import diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"

const (
	MaxSourceBytes = 250 * 1024
	MaxElements    = 250
	MaxConnectors  = 500
)

type Direction string

const (
	DirectionTB Direction = "TB"
	DirectionTD Direction = "TD"
	DirectionBT Direction = "BT"
	DirectionRL Direction = "RL"
	DirectionLR Direction = "LR"
)

type ParsedDiagram struct {
	Elements   []*diagv1.PlanElement
	Connectors []*diagv1.PlanConnector
	Warnings   []string
	Direction  Direction
	Source     string
}

func DirectionFromString(value string) Direction {
	switch value {
	case "TB":
		return DirectionTB
	case "TD":
		return DirectionTD
	case "BT":
		return DirectionBT
	case "RL":
		return DirectionRL
	case "LR":
		return DirectionLR
	default:
		return DirectionLR
	}
}

func DirectionToProto(value Direction) diagv1.MermaidDirection {
	switch value {
	case DirectionTB:
		return diagv1.MermaidDirection_MERMAID_DIRECTION_TB
	case DirectionTD:
		return diagv1.MermaidDirection_MERMAID_DIRECTION_TD
	case DirectionBT:
		return diagv1.MermaidDirection_MERMAID_DIRECTION_BT
	case DirectionRL:
		return diagv1.MermaidDirection_MERMAID_DIRECTION_RL
	case DirectionLR:
		return diagv1.MermaidDirection_MERMAID_DIRECTION_LR
	default:
		return diagv1.MermaidDirection_MERMAID_DIRECTION_LR
	}
}

func DirectionFromProto(value diagv1.MermaidDirection) Direction {
	switch value {
	case diagv1.MermaidDirection_MERMAID_DIRECTION_TB:
		return DirectionTB
	case diagv1.MermaidDirection_MERMAID_DIRECTION_TD:
		return DirectionTD
	case diagv1.MermaidDirection_MERMAID_DIRECTION_BT:
		return DirectionBT
	case diagv1.MermaidDirection_MERMAID_DIRECTION_RL:
		return DirectionRL
	case diagv1.MermaidDirection_MERMAID_DIRECTION_LR:
		return DirectionLR
	default:
		return DirectionLR
	}
}

func ptrString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
