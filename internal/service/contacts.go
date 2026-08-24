package service

import (
	"context"
	"time"

	"github.com/VamsiKrishna1211/applytrail/internal/dedupe"
	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
)

// UpsertContactRequest records a person and optionally links them to an
// application.
type UpsertContactRequest struct {
	Contact domain.Contact
	// LinkToApplicationID, when set, associates the contact with that
	// application and records the interaction on its timeline.
	LinkToApplicationID *domain.ID
	// CompanyNormalizedName resolves the employer when the caller knows the
	// name but not the ID.
	CompanyName string
}

// UpsertContact records a contact, keyed on email when one is supplied.
func (s *Service) UpsertContact(ctx context.Context, p domain.Principal, req UpsertContactRequest) (domain.Contact, error) {
	if err := p.RequireScope(domain.ScopeWrite); err != nil {
		return domain.Contact{}, err
	}
	req.Contact.TenantID = p.TenantID
	if err := req.Contact.Validate(); err != nil {
		return domain.Contact{}, err
	}

	var result domain.Contact
	err := s.store.Tx(ctx, func(ctx context.Context, tx store.Store) error {
		if req.Contact.CompanyID == nil && req.CompanyName != "" {
			company, err := tx.Companies().GetByNormalizedName(ctx, p.TenantID, dedupe.NormalizeCompany(req.CompanyName))
			if err == nil {
				req.Contact.CompanyID = &company.ID
			}
			// A contact at a company with no postings on record is still worth
			// keeping, so an unresolved name is not an error.
		}

		contact, err := tx.Contacts().Upsert(ctx, req.Contact)
		if err != nil {
			return err
		}

		if req.LinkToApplicationID != nil {
			if _, err := tx.Applications().Get(ctx, p.TenantID, *req.LinkToApplicationID); err != nil {
				return err
			}
			if err := tx.Contacts().LinkToApplication(ctx, *req.LinkToApplicationID, contact.ID); err != nil {
				return err
			}
			if err := s.appendEvent(ctx, tx, p, *req.LinkToApplicationID, domain.ApplicationEvent{
				Type:       domain.EventContactLogged,
				OccurredAt: contactTime(contact, s.now()),
				Payload: jsonPayload(map[string]any{
					"contact_id": contact.ID.String(),
					"name":       contact.Name,
					"role":       contact.Role,
				}),
			}); err != nil {
				return err
			}
		}

		result = contact
		return nil
	})
	return result, err
}

// ListContacts returns contacts, optionally scoped to one company.
func (s *Service) ListContacts(ctx context.Context, p domain.Principal, companyID *domain.ID, limit int) ([]domain.Contact, error) {
	if err := p.RequireScope(domain.ScopeRead); err != nil {
		return nil, err
	}
	return s.store.Contacts().List(ctx, p.TenantID, companyID, limit)
}

func contactTime(c domain.Contact, fallback time.Time) time.Time {
	if c.LastContactAt != nil {
		return *c.LastContactAt
	}
	return fallback
}
