package data

import (
	"context"
	"time"

	"gitlab.calendaria.team/services/orders/ent"
	"gitlab.calendaria.team/services/orders/ent/enum"
	entorder "gitlab.calendaria.team/services/orders/ent/order"
	utils_v1 "gitlab.calendaria.team/services/utils/api/utils/v1"
)

type OrdersRepo interface {
	Create(ctx context.Context, dto OrderDto, items []OrderItemDto) (*ent.Order, error)
	Get(ctx context.Context, tenantID, id int64) (*ent.Order, error)
	UpdateStatus(ctx context.Context, tenantID, id int64, status enum.OrderStatus) (*ent.Order, error)
	List(ctx context.Context, tenantID int64, counterpartyID int64, status *enum.OrderStatus, paginate *utils_v1.PaginateRequest) ([]*ent.Order, error)
	Count(ctx context.Context, tenantID int64, counterpartyID int64, status *enum.OrderStatus, paginate *utils_v1.PaginateRequest) (int32, error)
}

type ordersRepo struct {
	db *ent.Client
}

func NewOrdersRepo(d *Data) OrdersRepo {
	return &ordersRepo{db: d.db}
}

func (r *ordersRepo) Create(ctx context.Context, dto OrderDto, items []OrderItemDto) (*ent.Order, error) {
	tx, err := r.db.Tx(ctx)
	if err != nil {
		return nil, err
	}

	o, err := tx.Order.Create().
		SetTenantID(dto.TenantID).
		SetCounterpartyID(dto.CounterpartyID).
		SetStatus(enum.Created).
		SetCreatedBy(dto.CreatedBy).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	for _, item := range items {
		_, err := tx.OrderItem.Create().
			SetOrderID(o.ID).
			SetProductID(item.ProductID).
			SetQuantity(item.Quantity).
			SetUnitPrice(item.UnitPrice).
			Save(ctx)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return r.Get(ctx, dto.TenantID, o.ID)
}

func (r *ordersRepo) Get(ctx context.Context, tenantID, id int64) (*ent.Order, error) {
	return r.db.Order.Query().
		Where(entorder.ID(id), entorder.TenantID(tenantID)).
		WithItems().
		Only(ctx)
}

func (r *ordersRepo) UpdateStatus(ctx context.Context, tenantID, id int64, status enum.OrderStatus) (*ent.Order, error) {
	o, err := r.db.Order.UpdateOneID(id).
		Where(entorder.TenantID(tenantID)).
		SetStatus(status).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	return r.db.Order.Query().
		Where(entorder.ID(o.ID)).
		WithItems().
		Only(ctx)
}

func (r *ordersRepo) listQuery(tenantID int64, counterpartyID int64, status *enum.OrderStatus, paginate *utils_v1.PaginateRequest) *ent.OrderQuery {
	query := r.db.Order.Query().Where(entorder.TenantID(tenantID))

	if counterpartyID != 0 {
		query = query.Where(entorder.CounterpartyID(counterpartyID))
	}

	if status != nil {
		query = query.Where(entorder.StatusEQ(*status))
	}

	if paginate != nil {
		if paginate.GetFromDate() != "" {
			if t, err := time.Parse("2006-01-02", paginate.GetFromDate()); err == nil {
				query = query.Where(entorder.CreatedAtGTE(t))
			}
		}
		if paginate.GetToDate() != "" {
			if t, err := time.Parse("2006-01-02", paginate.GetToDate()); err == nil {
				query = query.Where(entorder.CreatedAtLTE(t.Add(24*time.Hour - time.Nanosecond)))
			}
		}
	}

	return query
}

func (r *ordersRepo) List(ctx context.Context, tenantID int64, counterpartyID int64, status *enum.OrderStatus, paginate *utils_v1.PaginateRequest) ([]*ent.Order, error) {
	query := r.listQuery(tenantID, counterpartyID, status, paginate)

	if paginate != nil && paginate.GetFromId() != 0 {
		query = query.Where(entorder.IDLT(paginate.GetFromId()))
	}

	limit := 100
	if paginate != nil && paginate.GetLimit() > 0 {
		limit = int(paginate.GetLimit())
	}

	return query.WithItems().Limit(limit).Order(ent.Desc(entorder.FieldID)).All(ctx)
}

func (r *ordersRepo) Count(ctx context.Context, tenantID int64, counterpartyID int64, status *enum.OrderStatus, paginate *utils_v1.PaginateRequest) (int32, error) {
	count, err := r.listQuery(tenantID, counterpartyID, status, paginate).Count(ctx)
	return int32(count), err
}
