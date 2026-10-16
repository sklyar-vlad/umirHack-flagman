package errors

import "github.com/google/uuid"

const ErrUnitLocked CatalogConflict = "unit is locked by history"

type CatalogConflict string

func (e CatalogConflict) Error() string { return string(e) }
func (e CatalogConflict) Unwrap() error { return ErrConflict }

type DishReference struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}
type ProductInUse struct{ Dishes []DishReference }

func (e *ProductInUse) Error() string { return "product is used by an active dish" }
func (e *ProductInUse) Unwrap() error { return ErrConflict }
