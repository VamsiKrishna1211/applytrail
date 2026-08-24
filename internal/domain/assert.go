package domain

import "errors"

// asValidation is errors.As specialised to *ValidationError, kept unexported
// so the generic helper is not part of the package's public surface.
func asValidation(err error, target **ValidationError) bool {
	return errors.As(err, target)
}
