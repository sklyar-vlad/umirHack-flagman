package supply

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type preparedSupplyInput struct {
	Number     string
	ReceivedAt time.Time
	Items      []Line
	ProductIDs []uuid.UUID
}

func prepareSupplyInput(input dto.CreateSupplyRequest) (preparedSupplyInput, error) {
	number := strings.TrimSpace(input.Number)

	if !utf8.ValidString(number) ||
		strings.ContainsRune(number, '\x00') ||
		utf8.RuneCountInString(number) < 1 ||
		utf8.RuneCountInString(number) > 80 {
		return preparedSupplyInput{}, &validations.FieldError{
			Field: "number",
			Message: "Номер должен содержать от 1 до 80 символов " +
				"без NUL после удаления крайних пробелов",
		}
	}

	receivedAt, err := parseBatchDate(
		"received_at",
		input.ReceivedAt,
	)
	if err != nil {
		return preparedSupplyInput{}, err
	}

	if err := validations.ValidateCount(
		"items",
		len(input.Items),
	); err != nil {
		return preparedSupplyInput{}, err
	}

	items := make([]Line, len(input.Items))
	ids := make([]uuid.UUID, 0, len(input.Items))
	seen := make(map[uuid.UUID]struct{}, len(input.Items))

	for i, item := range input.Items {
		productField := fmt.Sprintf("items[%d].product_id", i)

		id, err := validations.ParseUUID(
			productField,
			item.ProductID,
		)
		if err != nil {
			return preparedSupplyInput{}, err
		}

		if id == uuid.Nil {
			return preparedSupplyInput{}, &validations.FieldError{
				Field:   productField,
				Message: "Нулевой UUID продукта недопустим",
			}
		}

		quantityField := fmt.Sprintf("items[%d].quantity", i)

		quantity, err := validations.ParseQuantity(
			quantityField,
			item.Quantity,
		)
		if err != nil {
			return preparedSupplyInput{}, err
		}

		if quantity == 0 {
			return preparedSupplyInput{}, &validations.FieldError{
				Field:   quantityField,
				Message: "Количество должно быть больше нуля",
			}
		}

		expiry, err := parseExpiryInput(fmt.Sprintf("items[%d]", i), item, receivedAt)
		if err != nil {
			return preparedSupplyInput{}, err
		}

		items[i] = Line{
			ProductID:     id,
			QuantityMilli: quantity,
			Expiry:        expiry,
		}

		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}

	return preparedSupplyInput{
		Number: number, ReceivedAt: receivedAt, Items: items, ProductIDs: ids,
	}, nil
}
