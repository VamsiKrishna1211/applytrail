package domain

import (
	"fmt"
	"time"
)

// ResurfaceNever marks a status whose matches are always duplicates.
const ResurfaceNever = -1

// ResurfacePolicy decides whether a previously seen posting is worth showing
// again, expressed as a cooldown in days per status.
//
// The asymmetry is the point: a role the tenant never judged is cheap to
// reconsider, one they deliberately skipped costs more, and one that rejected
// them should not come back at all.
type ResurfacePolicy struct {
	// CooldownDays maps a status to the number of days after which a match in
	// that status becomes eligible again. ResurfaceNever suppresses forever.
	CooldownDays map[Status]int
}

// DefaultResurfacePolicy is applied when a search profile does not override it.
func DefaultResurfacePolicy() ResurfacePolicy {
	return ResurfacePolicy{CooldownDays: map[Status]int{
		StatusDiscovered:  30,
		StatusScreenedOut: 90,
		StatusQueued:      ResurfaceNever, // still on the to-do list
		StatusGhosted:     120,
		StatusWithdrawn:   180,
		StatusApplied:     ResurfaceNever,
		StatusScreening:   ResurfaceNever,
		StatusInterview:   ResurfaceNever,
		StatusOffer:       ResurfaceNever,
		StatusAccepted:    ResurfaceNever,
		StatusRejected:    ResurfaceNever,
	}}
}

// CooldownFor returns the cooldown for status, falling back to the default
// policy for any status the profile does not mention.
func (p ResurfacePolicy) CooldownFor(status Status) int {
	if days, ok := p.CooldownDays[status]; ok {
		return days
	}
	return DefaultResurfacePolicy().CooldownDays[status]
}

// Eligible reports whether a match last seen at lastSeen, sitting in status,
// should be surfaced again as of now. It also returns the reason, phrased for
// a human or model reader.
func (p ResurfacePolicy) Eligible(status Status, lastSeen, now time.Time) (bool, string) {
	cooldown := p.CooldownFor(status)
	if cooldown == ResurfaceNever {
		return false, fmt.Sprintf("status %q never resurfaces", status)
	}

	elapsed := int(now.Sub(lastSeen).Hours() / 24)
	if elapsed >= cooldown {
		return true, fmt.Sprintf("last seen %dd ago in status %q, past the %dd cooldown", elapsed, status, cooldown)
	}
	return false, fmt.Sprintf("last seen %dd ago in status %q, within the %dd cooldown", elapsed, status, cooldown)
}

// Validate rejects nonsensical cooldowns.
func (p ResurfacePolicy) Validate() error {
	var v ValidationError
	for status, days := range p.CooldownDays {
		if !status.Valid() {
			v.Add("resurface policy references unknown status %q", status)
		}
		if days < 0 && days != ResurfaceNever {
			v.Add("resurface cooldown for %q must be >= 0 or %d (never), got %d", status, ResurfaceNever, days)
		}
	}
	return v.OrNil()
}
