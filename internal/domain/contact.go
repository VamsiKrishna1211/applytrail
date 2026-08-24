package domain

import "time"

// Contact is a person at a company: a recruiter, hiring manager, or referral.
// Contacts belong to a company rather than an application, because the same
// recruiter often handles several roles.
type Contact struct {
	ID        ID
	TenantID  ID
	CompanyID *ID

	Name        string
	Role        string
	Email       string
	LinkedInURL string
	Notes       string

	LastContactAt *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Validate reports every problem with the contact at once.
func (c Contact) Validate() error {
	var v ValidationError
	if c.Name == "" && c.Email == "" {
		v.Add("contact needs at least a name or an email")
	}
	return v.OrNil()
}
