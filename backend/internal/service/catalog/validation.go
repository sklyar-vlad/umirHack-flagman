package catalog

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

const unitPieces = "pcs"

func field(name string) error {
	return &validations.FieldError{Field: name, Message: "Некорректное значение"}
}

func name(raw string) (string, error) {
	if !utf8.ValidString(raw) {
		return "", field("name")
	}
	value := strings.TrimSpace(raw)
	if value == "" || utf8.RuneCountInString(value) > 200 {
		return "", field("name")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", field("name")
		}
	}
	return value, nil
}

func revision(value int) error {
	if value < 1 || value >= 2147483647 {
		return field("expected_revision")
	}
	return nil
}

func mutationError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		if pg.Code == "23505" &&
			(pg.ConstraintName == "products_name_ci_uidx" || pg.ConstraintName == "categories_name_ci_uidx") {
			return apperrors.ErrConflict
		}
		if pg.Code == "23503" && pg.ConstraintName == "products_category_fk" {
			return field("category_id")
		}
	}
	return databaseError(err)
}

func databaseError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return fmt.Errorf("catalog transaction: database error (SQLSTATE %s)", pg.Code)
	}
	return err
}

func validateProduct(p *model.Product) error {
	value, err := name(p.Name)
	if err != nil {
		return err
	}
	p.Name = value
	if p.Category.ID == uuid.Nil {
		return field("category_id")
	}
	compatible := false
	switch p.Unit {
	case "g":
		compatible = p.DisplayUnit == "g" || p.DisplayUnit == "kg"
	case "ml":
		compatible = p.DisplayUnit == "ml" || p.DisplayUnit == "l"
	case unitPieces:
		compatible = p.DisplayUnit == unitPieces
	default:
		return field("unit")
	}
	if !compatible {
		return field("display_unit")
	}
	if p.StandardShelfLifeHours != nil && (*p.StandardShelfLifeHours < 1 || *p.StandardShelfLifeHours > 2147483647) {
		return field("standard_shelf_life_hours")
	}
	if p.TargetStock != nil {
		value, err := validations.ParseQuantity("target_stock", *p.TargetStock)
		if err != nil {
			return err
		}
		if p.Unit == unitPieces && value%1000 != 0 {
			return field("target_stock")
		}
	}
	return nil
}
