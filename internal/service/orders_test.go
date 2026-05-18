package service

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	v1 "github.com/makesalekz/orders/api/orders/v1"
	"github.com/makesalekz/orders/ent"
	"github.com/makesalekz/orders/ent/enum"
	"github.com/makesalekz/orders/internal/biz"
	"github.com/makesalekz/orders/internal/data"
	utils_v1 "github.com/makesalekz/utils/api/utils/v1"
	"github.com/makesalekz/utils/v2/auth"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errNotFound = errors.New("not found")

// --- Mock OrdersRepo ---

type mockOrdersRepo struct {
	orders map[int64]*ent.Order
	items  map[int64]*ent.OrderItem
	nextID int64
	nextItemID int64
}

func newMockOrdersRepo() *mockOrdersRepo {
	return &mockOrdersRepo{
		orders: make(map[int64]*ent.Order),
		items:  make(map[int64]*ent.OrderItem),
		nextID: 1,
		nextItemID: 1,
	}
}

func (m *mockOrdersRepo) Create(_ context.Context, dto data.OrderDto, itemDtos []data.OrderItemDto) (*ent.Order, error) {
	o := &ent.Order{
		ID:             m.nextID,
		TenantID:       dto.TenantID,
		CounterpartyID: dto.CounterpartyID,
		Status:         enum.Created,
		CreatedBy:      dto.CreatedBy,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	m.nextID++

	var edgeItems []*ent.OrderItem
	for _, itemDto := range itemDtos {
		item := &ent.OrderItem{
			ID:        m.nextItemID,
			OrderID:   o.ID,
			ProductID: itemDto.ProductID,
			Quantity:  itemDto.Quantity,
			UnitPrice: itemDto.UnitPrice,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		m.items[m.nextItemID] = item
		m.nextItemID++
		edgeItems = append(edgeItems, item)
	}

	o.Edges.Items = edgeItems
	m.orders[o.ID] = o
	return o, nil
}

func (m *mockOrdersRepo) Get(_ context.Context, tenantID, id int64) (*ent.Order, error) {
	o, ok := m.orders[id]
	if !ok || o.TenantID != tenantID {
		return nil, errNotFound
	}
	return o, nil
}

func (m *mockOrdersRepo) UpdateStatus(_ context.Context, tenantID, id int64, status enum.OrderStatus) (*ent.Order, error) {
	o, ok := m.orders[id]
	if !ok || o.TenantID != tenantID {
		return nil, errNotFound
	}
	o.Status = status
	o.UpdatedAt = time.Now()
	return o, nil
}

func (m *mockOrdersRepo) List(_ context.Context, tenantID int64, counterpartyID int64, status *enum.OrderStatus, paginate *utils_v1.PaginateRequest) ([]*ent.Order, error) {
	var result []*ent.Order
	for _, o := range m.orders {
		if o.TenantID != tenantID {
			continue
		}
		if counterpartyID != 0 && o.CounterpartyID != counterpartyID {
			continue
		}
		if status != nil && o.Status != *status {
			continue
		}
		if paginate != nil && paginate.GetFromDate() != "" {
			if t, err := time.Parse("2006-01-02", paginate.GetFromDate()); err == nil {
				if o.CreatedAt.Before(t) {
					continue
				}
			}
		}
		if paginate != nil && paginate.GetToDate() != "" {
			if t, err := time.Parse("2006-01-02", paginate.GetToDate()); err == nil {
				if o.CreatedAt.After(t.Add(24*time.Hour - time.Nanosecond)) {
					continue
				}
			}
		}
		result = append(result, o)
	}

	// Sort by created_at desc
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	limit := 100
	if paginate != nil && paginate.GetLimit() > 0 {
		limit = int(paginate.GetLimit())
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (m *mockOrdersRepo) Count(_ context.Context, tenantID int64, counterpartyID int64, status *enum.OrderStatus, paginate *utils_v1.PaginateRequest) (int32, error) {
	var count int32
	for _, o := range m.orders {
		if o.TenantID != tenantID {
			continue
		}
		if counterpartyID != 0 && o.CounterpartyID != counterpartyID {
			continue
		}
		if status != nil && o.Status != *status {
			continue
		}
		if paginate != nil && paginate.GetFromDate() != "" {
			if t, err := time.Parse("2006-01-02", paginate.GetFromDate()); err == nil {
				if o.CreatedAt.Before(t) {
					continue
				}
			}
		}
		if paginate != nil && paginate.GetToDate() != "" {
			if t, err := time.Parse("2006-01-02", paginate.GetToDate()); err == nil {
				if o.CreatedAt.After(t.Add(24*time.Hour - time.Nanosecond)) {
					continue
				}
			}
		}
		count++
	}
	return count, nil
}

// --- Mock Publisher ---

type mockPublisher struct {
	events []data.OrderConfirmedEvent
}

func newMockPublisher() *mockPublisher {
	return &mockPublisher{}
}

func (p *mockPublisher) Publish(event data.OrderConfirmedEvent) {
	p.events = append(p.events, event)
}

// --- Test setup ---

func setupService() (*OrdersService, *mockOrdersRepo, *mockPublisher) {
	repo := newMockOrdersRepo()
	pub := newMockPublisher()
	uc := biz.NewOrdersUsecase(log.DefaultLogger, repo, pub)
	svc := NewOrdersService(uc)
	return svc, repo, pub
}

func ctxWithTenant(tenantID int64) context.Context {
	return auth.NewTenantContext(context.Background(), tenantID)
}

func ctxWithTenantAndActor(tenantID, actorID int64) context.Context {
	ctx := auth.NewTenantContext(context.Background(), tenantID)
	return auth.NewActorContext(ctx, actorID)
}

// ============================================================
// Story 5.1: Create Order
// ============================================================

func TestCreateOrder(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100.50"},
			{ProductId: 20, Quantity: "3", UnitPrice: "200"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Order)
	assert.Equal(t, int64(1), resp.Order.TenantId)
	assert.Equal(t, int64(100), resp.Order.CounterpartyId)
	assert.Equal(t, v1.OrderStatus_CREATED, resp.Order.Status)
	assert.Equal(t, int64(42), resp.Order.CreatedBy)
	assert.Len(t, resp.Order.Items, 2)
	assert.Equal(t, int64(10), resp.Order.Items[0].ProductId)
	assert.Equal(t, "5", resp.Order.Items[0].Quantity)
	assert.Equal(t, "100.5", resp.Order.Items[0].UnitPrice)
}

func TestCreateOrder_NoTenant(t *testing.T) {
	svc, _, _ := setupService()
	ctx := context.Background()

	_, err := svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100"},
		},
	})
	require.Error(t, err)
}

func TestCreateOrder_NoCounterparty(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenant(1)

	_, err := svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100"},
		},
	})
	require.Error(t, err)
}

func TestCreateOrder_NoItems(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenant(1)

	_, err := svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 100,
	})
	require.Error(t, err)
}

func TestGetOrder(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenant(1)

	created, _ := svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100"},
		},
	})

	resp, err := svc.GetOrder(ctx, &v1.GetOrderRequest{Id: created.Order.Id})
	require.NoError(t, err)
	assert.Equal(t, created.Order.Id, resp.Order.Id)
	assert.Len(t, resp.Order.Items, 1)
}

func TestGetOrder_NotFound(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenant(1)

	_, err := svc.GetOrder(ctx, &v1.GetOrderRequest{Id: 999})
	require.Error(t, err)
}

func TestGetOrder_TenantIsolation(t *testing.T) {
	svc, _, _ := setupService()
	ctx1 := ctxWithTenant(1)
	ctx2 := ctxWithTenant(2)

	created, _ := svc.CreateOrder(ctx1, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100"},
		},
	})

	_, err := svc.GetOrder(ctx2, &v1.GetOrderRequest{Id: created.Order.Id})
	require.Error(t, err)
}

// ============================================================
// Story 5.2: Status Transitions
// ============================================================

func TestConfirmOrder(t *testing.T) {
	svc, _, pub := setupService()
	ctx := ctxWithTenant(1)

	created, _ := svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100"},
		},
	})

	resp, err := svc.ConfirmOrder(ctx, &v1.ConfirmOrderRequest{Id: created.Order.Id})
	require.NoError(t, err)
	assert.Equal(t, v1.OrderStatus_CONFIRMED, resp.Order.Status)

	// Verify NATS event was published
	require.Len(t, pub.events, 1)
	assert.Equal(t, int64(1), pub.events[0].TenantID)
	assert.Equal(t, created.Order.Id, pub.events[0].OrderID)
	assert.Equal(t, int64(100), pub.events[0].CounterpartyID)
}

func TestShipOrder(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenant(1)

	created, _ := svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100"},
		},
	})

	svc.ConfirmOrder(ctx, &v1.ConfirmOrderRequest{Id: created.Order.Id})
	resp, err := svc.ShipOrder(ctx, &v1.ShipOrderRequest{Id: created.Order.Id})
	require.NoError(t, err)
	assert.Equal(t, v1.OrderStatus_SHIPPED, resp.Order.Status)
}

func TestAcceptOrder(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenant(1)

	created, _ := svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100"},
		},
	})

	svc.ConfirmOrder(ctx, &v1.ConfirmOrderRequest{Id: created.Order.Id})
	svc.ShipOrder(ctx, &v1.ShipOrderRequest{Id: created.Order.Id})
	resp, err := svc.AcceptOrder(ctx, &v1.AcceptOrderRequest{Id: created.Order.Id})
	require.NoError(t, err)
	assert.Equal(t, v1.OrderStatus_ACCEPTED, resp.Order.Status)
}

func TestRejectOrder(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenant(1)

	created, _ := svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100"},
		},
	})

	resp, err := svc.RejectOrder(ctx, &v1.RejectOrderRequest{Id: created.Order.Id})
	require.NoError(t, err)
	assert.Equal(t, v1.OrderStatus_REJECTED, resp.Order.Status)
}

func TestFullLifecycle_Created_Confirmed_Shipped_Accepted(t *testing.T) {
	svc, _, pub := setupService()
	ctx := ctxWithTenant(1)

	created, err := svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, v1.OrderStatus_CREATED, created.Order.Status)

	confirmed, err := svc.ConfirmOrder(ctx, &v1.ConfirmOrderRequest{Id: created.Order.Id})
	require.NoError(t, err)
	assert.Equal(t, v1.OrderStatus_CONFIRMED, confirmed.Order.Status)

	shipped, err := svc.ShipOrder(ctx, &v1.ShipOrderRequest{Id: created.Order.Id})
	require.NoError(t, err)
	assert.Equal(t, v1.OrderStatus_SHIPPED, shipped.Order.Status)

	accepted, err := svc.AcceptOrder(ctx, &v1.AcceptOrderRequest{Id: created.Order.Id})
	require.NoError(t, err)
	assert.Equal(t, v1.OrderStatus_ACCEPTED, accepted.Order.Status)

	// Only 1 event should have been published (on CONFIRMED)
	assert.Len(t, pub.events, 1)
}

// --- Invalid transitions ---

func TestConfirmOrder_AlreadyConfirmed(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenant(1)

	created, _ := svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100"},
		},
	})

	svc.ConfirmOrder(ctx, &v1.ConfirmOrderRequest{Id: created.Order.Id})

	// Trying to confirm again should fail
	_, err := svc.ConfirmOrder(ctx, &v1.ConfirmOrderRequest{Id: created.Order.Id})
	require.Error(t, err)
}

func TestShipOrder_FromCreated(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenant(1)

	created, _ := svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100"},
		},
	})

	// Cannot ship from CREATED status
	_, err := svc.ShipOrder(ctx, &v1.ShipOrderRequest{Id: created.Order.Id})
	require.Error(t, err)
}

func TestAcceptOrder_FromCreated(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenant(1)

	created, _ := svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100"},
		},
	})

	_, err := svc.AcceptOrder(ctx, &v1.AcceptOrderRequest{Id: created.Order.Id})
	require.Error(t, err)
}

func TestRejectOrder_FromConfirmed(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenant(1)

	created, _ := svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100"},
		},
	})

	svc.ConfirmOrder(ctx, &v1.ConfirmOrderRequest{Id: created.Order.Id})

	// Cannot reject from CONFIRMED (only from CREATED)
	_, err := svc.RejectOrder(ctx, &v1.RejectOrderRequest{Id: created.Order.Id})
	require.Error(t, err)
}

func TestRejectOrder_FromShipped(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenant(1)

	created, _ := svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100"},
		},
	})

	svc.ConfirmOrder(ctx, &v1.ConfirmOrderRequest{Id: created.Order.Id})
	svc.ShipOrder(ctx, &v1.ShipOrderRequest{Id: created.Order.Id})

	// Cannot reject from SHIPPED
	_, err := svc.RejectOrder(ctx, &v1.RejectOrderRequest{Id: created.Order.Id})
	require.Error(t, err)
}

func TestRejectOrder_FromAccepted(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenant(1)

	created, _ := svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100"},
		},
	})

	svc.ConfirmOrder(ctx, &v1.ConfirmOrderRequest{Id: created.Order.Id})
	svc.ShipOrder(ctx, &v1.ShipOrderRequest{Id: created.Order.Id})
	svc.AcceptOrder(ctx, &v1.AcceptOrderRequest{Id: created.Order.Id})

	// Cannot reject from ACCEPTED (terminal)
	_, err := svc.RejectOrder(ctx, &v1.RejectOrderRequest{Id: created.Order.Id})
	require.Error(t, err)
}

func TestConfirmOrder_NotFound(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenant(1)

	_, err := svc.ConfirmOrder(ctx, &v1.ConfirmOrderRequest{Id: 999})
	require.Error(t, err)
}

func TestStatusTransition_TenantIsolation(t *testing.T) {
	svc, _, _ := setupService()
	ctx1 := ctxWithTenant(1)
	ctx2 := ctxWithTenant(2)

	created, _ := svc.CreateOrder(ctx1, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100"},
		},
	})

	// Tenant 2 should not be able to confirm tenant 1's order
	_, err := svc.ConfirmOrder(ctx2, &v1.ConfirmOrderRequest{Id: created.Order.Id})
	require.Error(t, err)
}

func TestConfirmOrder_NoTenant(t *testing.T) {
	svc, _, _ := setupService()
	ctx := context.Background()

	_, err := svc.ConfirmOrder(ctx, &v1.ConfirmOrderRequest{Id: 1})
	require.Error(t, err)
}

// ============================================================
// Story 5.3: Order Suggestions
// ============================================================

func TestGetOrderSuggestions(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenant(1)

	resp, err := svc.GetOrderSuggestions(ctx, &v1.GetOrderSuggestionsRequest{WarehouseId: 1})
	require.NoError(t, err)
	require.NotNil(t, resp)
	// Placeholder returns empty list
	assert.Empty(t, resp.Items)
}

func TestGetOrderSuggestions_NoTenant(t *testing.T) {
	svc, _, _ := setupService()
	ctx := context.Background()

	_, err := svc.GetOrderSuggestions(ctx, &v1.GetOrderSuggestionsRequest{WarehouseId: 1})
	require.Error(t, err)
}

// ============================================================
// Story 5.4: List Orders / Order History
// ============================================================

func TestListOrders(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenant(1)

	svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100"},
		},
	})
	svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 200,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 20, Quantity: "3", UnitPrice: "200"},
		},
	})

	resp, err := svc.ListOrders(ctx, &v1.ListOrdersRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 2)
	assert.NotNil(t, resp.Paginate)
	assert.Equal(t, int32(2), *resp.Paginate.Total)
}

func TestListOrders_TenantIsolation(t *testing.T) {
	svc, _, _ := setupService()
	ctx1 := ctxWithTenant(1)
	ctx2 := ctxWithTenant(2)

	svc.CreateOrder(ctx1, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100"},
		},
	})
	svc.CreateOrder(ctx2, &v1.CreateOrderRequest{
		CounterpartyId: 200,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 20, Quantity: "3", UnitPrice: "200"},
		},
	})

	resp, err := svc.ListOrders(ctx1, &v1.ListOrdersRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 1)
}

func TestListOrders_FilterByCounterparty(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenant(1)

	svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100"},
		},
	})
	svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 200,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 20, Quantity: "3", UnitPrice: "200"},
		},
	})

	resp, err := svc.ListOrders(ctx, &v1.ListOrdersRequest{CounterpartyId: 100})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, int64(100), resp.Items[0].CounterpartyId)
}

func TestListOrders_FilterByStatus(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenant(1)

	created1, _ := svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100"},
		},
	})
	svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 200,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 20, Quantity: "3", UnitPrice: "200"},
		},
	})

	// Confirm first order
	svc.ConfirmOrder(ctx, &v1.ConfirmOrderRequest{Id: created1.Order.Id})

	// Filter by CONFIRMED
	resp, err := svc.ListOrders(ctx, &v1.ListOrdersRequest{
		Status:       v1.OrderStatus_CONFIRMED,
		FilterStatus: true,
	})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, v1.OrderStatus_CONFIRMED, resp.Items[0].Status)
}

func TestListOrders_Pagination(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenant(1)

	for i := 0; i < 5; i++ {
		svc.CreateOrder(ctx, &v1.CreateOrderRequest{
			CounterpartyId: 100,
			Items: []*v1.CreateOrderItemRequest{
				{ProductId: int64(10 + i), Quantity: "1", UnitPrice: "100"},
			},
		})
	}

	resp, err := svc.ListOrders(ctx, &v1.ListOrdersRequest{
		Paginate: &utils_v1.PaginateRequest{Limit: 2},
	})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 2)
	assert.Equal(t, int32(5), *resp.Paginate.Total)
}

func TestListOrders_NoTenant(t *testing.T) {
	svc, _, _ := setupService()
	ctx := context.Background()

	_, err := svc.ListOrders(ctx, &v1.ListOrdersRequest{})
	require.Error(t, err)
}

func TestListOrders_IncludesItems(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenant(1)

	svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5", UnitPrice: "100"},
			{ProductId: 20, Quantity: "3", UnitPrice: "200"},
		},
	})

	resp, err := svc.ListOrders(ctx, &v1.ListOrdersRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	assert.Len(t, resp.Items[0].Items, 2)
}

// --- Response format tests ---

func TestOrderResponse_Format(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.CreateOrder(ctx, &v1.CreateOrderRequest{
		CounterpartyId: 100,
		Items: []*v1.CreateOrderItemRequest{
			{ProductId: 10, Quantity: "5.5", UnitPrice: "100.25"},
		},
	})
	require.NoError(t, err)

	order := resp.Order
	assert.True(t, order.Id > 0)
	assert.Equal(t, int64(1), order.TenantId)
	assert.Equal(t, int64(100), order.CounterpartyId)
	assert.Equal(t, v1.OrderStatus_CREATED, order.Status)
	assert.Equal(t, int64(42), order.CreatedBy)
	assert.NotEmpty(t, order.CreatedAt)
	assert.NotEmpty(t, order.UpdatedAt)

	item := order.Items[0]
	assert.True(t, item.Id > 0)
	assert.Equal(t, order.Id, item.OrderId)
	assert.Equal(t, int64(10), item.ProductId)
	assert.Equal(t, "5.5", item.Quantity)
	assert.Equal(t, "100.25", item.UnitPrice)
}

func TestParseDecimal(t *testing.T) {
	tests := []struct {
		input    string
		expected decimal.Decimal
	}{
		{"100.50", decimal.NewFromFloat(100.50)},
		{"0", decimal.Zero},
		{"", decimal.Zero},
		{"invalid", decimal.Zero},
	}

	for _, tt := range tests {
		result := parseDecimal(tt.input)
		assert.True(t, tt.expected.Equal(result), "parseDecimal(%q) = %s, want %s", tt.input, result, tt.expected)
	}
}
