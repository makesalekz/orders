package biz

import (
	"context"
	"fmt"

	"gitlab.calendaria.team/services/orders/ent"
	"gitlab.calendaria.team/services/orders/ent/enum"
	"gitlab.calendaria.team/services/orders/internal/data"
	utils_v1 "gitlab.calendaria.team/services/utils/api/utils/v1"

	"github.com/go-kratos/kratos/v2/log"
)

// validTransitions defines allowed status transitions.
var validTransitions = map[enum.OrderStatus][]enum.OrderStatus{
	enum.Created:   {enum.Confirmed, enum.Rejected},
	enum.Confirmed: {enum.Shipped},
	enum.Shipped:   {enum.Accepted},
	// Accepted and Rejected are terminal states.
}

type OrdersUsecase struct {
	log       *log.Helper
	repo      data.OrdersRepo
	publisher data.OrderConfirmedPublisher
}

func NewOrdersUsecase(logger log.Logger, repo data.OrdersRepo, publisher data.OrderConfirmedPublisher) *OrdersUsecase {
	return &OrdersUsecase{
		log:       log.NewHelper(logger),
		repo:      repo,
		publisher: publisher,
	}
}

func (uc *OrdersUsecase) Create(ctx context.Context, dto data.OrderDto, items []data.OrderItemDto) (*ent.Order, error) {
	return uc.repo.Create(ctx, dto, items)
}

func (uc *OrdersUsecase) Get(ctx context.Context, tenantID, id int64) (*ent.Order, error) {
	return uc.repo.Get(ctx, tenantID, id)
}

func (uc *OrdersUsecase) transition(ctx context.Context, tenantID, id int64, target enum.OrderStatus) (*ent.Order, error) {
	order, err := uc.repo.Get(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	allowed, ok := validTransitions[order.Status]
	if !ok {
		return nil, fmt.Errorf("no transitions from status %s", order.Status)
	}

	valid := false
	for _, s := range allowed {
		if s == target {
			valid = true
			break
		}
	}
	if !valid {
		return nil, fmt.Errorf("cannot transition from %s to %s", order.Status, target)
	}

	updated, err := uc.repo.UpdateStatus(ctx, tenantID, id, target)
	if err != nil {
		return nil, err
	}

	// Publish event on CONFIRMED
	if target == enum.Confirmed {
		uc.publisher.Publish(data.OrderConfirmedEvent{
			TenantID:       updated.TenantID,
			OrderID:        updated.ID,
			CounterpartyID: updated.CounterpartyID,
		})
	}

	return updated, nil
}

func (uc *OrdersUsecase) Confirm(ctx context.Context, tenantID, id int64) (*ent.Order, error) {
	return uc.transition(ctx, tenantID, id, enum.Confirmed)
}

func (uc *OrdersUsecase) Ship(ctx context.Context, tenantID, id int64) (*ent.Order, error) {
	return uc.transition(ctx, tenantID, id, enum.Shipped)
}

func (uc *OrdersUsecase) Accept(ctx context.Context, tenantID, id int64) (*ent.Order, error) {
	return uc.transition(ctx, tenantID, id, enum.Accepted)
}

func (uc *OrdersUsecase) Reject(ctx context.Context, tenantID, id int64) (*ent.Order, error) {
	return uc.transition(ctx, tenantID, id, enum.Rejected)
}

func (uc *OrdersUsecase) List(ctx context.Context, tenantID int64, counterpartyID int64, status *enum.OrderStatus, paginate *utils_v1.PaginateRequest) ([]*ent.Order, int32, error) {
	items, err := uc.repo.List(ctx, tenantID, counterpartyID, status, paginate)
	if err != nil {
		return nil, 0, err
	}

	total, err := uc.repo.Count(ctx, tenantID, counterpartyID, status, paginate)
	if err != nil {
		return nil, 0, err
	}

	return items, total, nil
}
