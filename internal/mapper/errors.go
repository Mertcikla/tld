package mapper

import "errors"

var (
	errVectorsNotRectangular = errors.New("vectors must be rectangular")
	errFactsVectorsMismatch  = errors.New("facts and vectors must have matching lengths")
	errInvalidFacts          = errors.New("facts require unique nonempty IDs and nonempty paths")
	errDuplicateMembership   = errors.New("duplicate or out-of-range cluster membership")
	errNotPartitioned        = errors.New("pipeline result must partition the dataset exactly once")
	errClusterSizeViolation  = errors.New("cluster violates size bounds")
)
