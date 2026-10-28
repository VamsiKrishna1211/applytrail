package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/store/postgres/sqlcgen"
)

type eventStore struct{ q *sqlcgen.Queries }

func (s eventStore) Insert(ctx context.Context, e domain.ApplicationEvent) (domain.ApplicationEvent, error) {
	occurredAt := e.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	row, err := s.q.InsertApplicationEvent(ctx, sqlcgen.InsertApplicationEventParams{
		TenantID:      e.TenantID,
		ApplicationID: e.ApplicationID,
		Type:          string(e.Type),
		FromStatus:    fromStatusPtr(e.FromStatus),
		ToStatus:      fromStatusPtr(e.ToStatus),
		OccurredAt:    occurredAt,
		Actor:         e.Actor,
		Payload:       orEmptyJSON(e.Payload),
	})
	if err != nil {
		return domain.ApplicationEvent{}, translate("insert application event", "application event", err)
	}
	return toDomainEvent(row), nil
}

func (s eventStore) List(ctx context.Context, tenantID, applicationID domain.ID, limit int) ([]domain.ApplicationEvent, error) {
	rows, err := s.q.ListApplicationEvents(ctx, sqlcgen.ListApplicationEventsParams{
		TenantID:      tenantID,
		ApplicationID: applicationID,
		RowLimit:      clampLimit(limit),
	})
	if err != nil {
		return nil, translate("list application events", "application event", err)
	}
	events := make([]domain.ApplicationEvent, len(rows))
	for i, row := range rows {
		events[i] = toDomainEvent(row)
	}
	return events, nil
}

func (s eventStore) LatestStatusChange(ctx context.Context, tenantID, applicationID domain.ID) (domain.ApplicationEvent, error) {
	row, err := s.q.GetLatestStatusChange(ctx, sqlcgen.GetLatestStatusChangeParams{
		TenantID:      tenantID,
		ApplicationID: applicationID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ApplicationEvent{}, notFound("status change for application", applicationID)
	}
	if err != nil {
		return domain.ApplicationEvent{}, translate("get latest status change", "application event", err)
	}
	return toDomainEvent(row), nil
}

type contactStore struct{ q *sqlcgen.Queries }

func (s contactStore) Upsert(ctx context.Context, c domain.Contact) (domain.Contact, error) {
	row, err := s.q.UpsertContactByEmail(ctx, sqlcgen.UpsertContactByEmailParams{
		TenantID:      c.TenantID,
		CompanyID:     c.CompanyID,
		Name:          c.Name,
		Role:          c.Role,
		Email:         c.Email,
		LinkedinUrl:   c.LinkedInURL,
		Notes:         c.Notes,
		LastContactAt: c.LastContactAt,
	})
	if err != nil {
		return domain.Contact{}, translate("upsert contact", "contact", err)
	}
	return toDomainContact(row), nil
}

func (s contactStore) Get(ctx context.Context, tenantID, id domain.ID) (domain.Contact, error) {
	row, err := s.q.GetContact(ctx, sqlcgen.GetContactParams{ID: id, TenantID: tenantID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Contact{}, notFound("contact", id)
	}
	if err != nil {
		return domain.Contact{}, translate("get contact", "contact", err)
	}
	return toDomainContact(row), nil
}

func (s contactStore) List(ctx context.Context, tenantID domain.ID, companyID *domain.ID, limit int) ([]domain.Contact, error) {
	rows, err := s.q.ListContacts(ctx, sqlcgen.ListContactsParams{
		TenantID:  tenantID,
		CompanyID: companyID,
		RowLimit:  clampLimit(limit),
	})
	if err != nil {
		return nil, translate("list contacts", "contact", err)
	}
	contacts := make([]domain.Contact, len(rows))
	for i, row := range rows {
		contacts[i] = toDomainContact(row)
	}
	return contacts, nil
}

func (s contactStore) LinkToApplication(ctx context.Context, tenantID, applicationID, contactID domain.ID) error {
	err := s.q.LinkContactToApplication(ctx, sqlcgen.LinkContactToApplicationParams{
		TenantID:      tenantID,
		ApplicationID: applicationID,
		ContactID:     contactID,
	})
	return translate("link contact to application", "contact", err)
}

func (s contactStore) ListForApplication(ctx context.Context, tenantID, applicationID domain.ID) ([]domain.Contact, error) {
	rows, err := s.q.ListContactsForApplication(ctx, sqlcgen.ListContactsForApplicationParams{
		ApplicationID: applicationID,
		TenantID:      tenantID,
	})
	if err != nil {
		return nil, translate("list contacts for application", "contact", err)
	}
	contacts := make([]domain.Contact, len(rows))
	for i, row := range rows {
		contacts[i] = toDomainContact(row)
	}
	return contacts, nil
}
