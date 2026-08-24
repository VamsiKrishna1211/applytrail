package service

import (
	"errors"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

// asValidation is errors.As specialised to *domain.ValidationError.
func asValidation(err error, target **domain.ValidationError) bool {
	return errors.As(err, target)
}
