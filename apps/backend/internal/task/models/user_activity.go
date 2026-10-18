package models

import "time"

type UserActivitySummary struct {
	SessionID        string
	ActiveMS         int64
	IntervalCount    int64
	FirstAt          time.Time
	LastAt           time.Time
	ActivityMeasured bool
}

// SessionActivityFilter narrows activity and turn reads by session or task.
type SessionActivityFilter struct {
	SessionIDs []string
	TaskIDs    []string
}

// SessionActivitySession is one keyset-ordered session selection row.
type SessionActivitySession struct {
	SessionID string
	TaskID    string
	StartedAt time.Time
}

// SessionActivitySessionsPage contains one bounded session-selection page.
type SessionActivitySessionsPage struct {
	Sessions   []SessionActivitySession
	NextCursor string
	HasMore    bool
}

// SessionActivityTurn contains only turn identity and lifecycle timestamps.
type SessionActivityTurn struct {
	ID          string
	SessionID   string
	TaskID      string
	StartedAt   time.Time
	CompletedAt *time.Time
}

// SessionActivityTurnsPage contains one bounded turn page.
type SessionActivityTurnsPage struct {
	Turns      []SessionActivityTurn
	NextCursor string
	HasMore    bool
}
