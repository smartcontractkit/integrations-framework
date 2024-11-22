package wasp

import (
	"time"
)

/* Different load profile schedules definitions */

const (
	// DefaultStepChangePrecision is default amount of steps in which we split a schedule
	DefaultStepChangePrecision = 10
)

// Plain create a constant workload Segment
func Plain(from int64, duration time.Duration) []*Segment {
	return []*Segment{
		{
			From:     from,
			Duration: duration,
		},
	}
}

// Steps creates a series of increasing/decreasing Segments
func Steps(from, increase int64, steps int, duration time.Duration) []*Segment {
	if from == 0 {
		from = 1
	}
	if steps == 0 {
		panic("steps can't be 0 in wasp.Steps schedule")
	}
	segments := make([]*Segment, 0)
	perStepDuration := duration / time.Duration(steps)
	for i := 0; i < steps; i++ {
		newFrom := from + int64(i)*increase
		segments = append(segments, &Segment{
			From:     newFrom,
			Duration: perStepDuration,
		})
	}
	return segments
}

// Combine combines profile segments
func Combine(segs ...[]*Segment) []*Segment {
	acc := make([]*Segment, 0)
	for _, ss := range segs {
		acc = append(acc, ss...)
	}
	return acc
}

// CombineAndRepeat combines and repeats profile segments
func CombineAndRepeat(times int, segs ...[]*Segment) []*Segment {
	if len(segs) == 0 {
		panic(ErrNoSchedule)
	}
	acc := make([]*Segment, 0)
	for i := 0; i < times; i++ {
		for _, ss := range segs {
			acc = append(acc, ss...)
		}
	}
	return acc
}
