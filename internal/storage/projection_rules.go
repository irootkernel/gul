package storage

import "github.com/rootkernel/gul/internal/observation"

var projectionKinds = [...]string{"run", "writer", "interaction"}

func coversProjectionFloor(kind string, stamp, floor observation.Stamp) bool {
	if kind == "writer" {
		return stamp.Writer >= floor.Writer
	}
	return stamp.Valid() && stamp.Covers(floor)
}
