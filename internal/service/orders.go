package service

import (
	"context"

	v1 "gitlab.calendaria.team/services/orders/api/orders/v1"
	"gitlab.calendaria.team/services/orders/ent"
	"gitlab.calendaria.team/services/orders/ent/enum"
	"gitlab.calendaria.team/services/orders/internal/biz"
	"gitlab.calendaria.team/services/orders/internal/data"
	utils_v1 "gitlab.calendaria.team/services/utils/api/utils/v1"
	"gitlab.calendaria.team/services/utils/v2/auth"

	"github.com/shopspring/decimal"
)

type OrdersService struct {
	v1.UnimplementedOrdersServiceServer

	uc *biz.OrdersUsecase
}

func NewOrdersService(uc *biz.OrdersUsecase) *OrdersService {
	return &OrdersService{uc: uc}
}

// --- 5.1: Create Order ---

func (s *OrdersService) CreateOrder(ctx context.Context, req *v1.CreateOrderRequest) (*v1.CreateOrderReply, error) {
	tenantID := auth.GetTenantIdFromContext(ctx)
	if tenantID == 0 {
		return nil, v1.ErrorInvalidRequest("empty tenant id")
	}

	if req.GetCounterpartyId() == 0 {
		return nil, v1.ErrorInvalidRequest("counterparty_id is required")
	}

	if len(req.GetItems()) == 0 {
		return nil, v1.ErrorInvalidRequest("at least one item is required")
	}

	actorID := auth.GetActorIdFromContext(ctx)

	dto := data.OrderDto{
		TenantID:       tenantID,
		CounterpartyID: req.GetCounterpartyId(),
		CreatedBy:      actorID,
	}

	items := make([]data.OrderItemDto, 0, len(req.GetItems()))
	for _, item := range req.GetItems() {
		items = append(items, data.OrderItemDto{
			ProductID: item.GetProductId(),
			Quantity:  parseDecimal(item.GetQuantity()),
			UnitPrice: parseDecimal(item.GetUnitPrice()),
		})
	}

	order, err := s.uc.Create(ctx, dto, items)
	if err != nil {
		return nil, err
	}

	return &v1.CreateOrderReply{Order: replyOrder(order)}, nil
}

func (s *OrdersService) GetOrder(ctx context.Context, req *v1.GetOrderRequest) (*v1.GetOrderReply, error) {
	tenantID := auth.GetTenantIdFromContext(ctx)
	if tenantID == 0 {
		return nil, v1.ErrorInvalidRequest("empty tenant id")
	}

	order, err := s.uc.Get(ctx, tenantID, req.GetId())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, v1.ErrorNotFound("order not found")
		}
		return nil, err
	}

	return &v1.GetOrderReply{Order: replyOrder(order)}, nil
}

// --- 5.2: Status transitions ---

func (s *OrdersService) ConfirmOrder(ctx context.Context, req *v1.ConfirmOrderRequest) (*v1.ConfirmOrderReply, error) {
	tenantID := auth.GetTenantIdFromContext(ctx)
	if tenantID == 0 {
		return nil, v1.ErrorInvalidRequest("empty tenant id")
	}

	order, err := s.uc.Confirm(ctx, tenantID, req.GetId())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, v1.ErrorNotFound("order not found")
		}
		return nil, v1.ErrorInvalidStatusTransition("%s", err.Error())
	}

	return &v1.ConfirmOrderReply{Order: replyOrder(order)}, nil
}

func (s *OrdersService) ShipOrder(ctx context.Context, req *v1.ShipOrderRequest) (*v1.ShipOrderReply, error) {
	tenantID := auth.GetTenantIdFromContext(ctx)
	if tenantID == 0 {
		return nil, v1.ErrorInvalidRequest("empty tenant id")
	}

	order, err := s.uc.Ship(ctx, tenantID, req.GetId())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, v1.ErrorNotFound("order not found")
		}
		return nil, v1.ErrorInvalidStatusTransition("%s", err.Error())
	}

	return &v1.ShipOrderReply{Order: replyOrder(order)}, nil
}

func (s *OrdersService) AcceptOrder(ctx context.Context, req *v1.AcceptOrderRequest) (*v1.AcceptOrderReply, error) {
	tenantID := auth.GetTenantIdFromContext(ctx)
	if tenantID == 0 {
		return nil, v1.ErrorInvalidRequest("empty tenant id")
	}

	order, err := s.uc.Accept(ctx, tenantID, req.GetId())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, v1.ErrorNotFound("order not found")
		}
		return nil, v1.ErrorInvalidStatusTransition("%s", err.Error())
	}

	return &v1.AcceptOrderReply{Order: replyOrder(order)}, nil
}

func (s *OrdersService) RejectOrder(ctx context.Context, req *v1.RejectOrderRequest) (*v1.RejectOrderReply, error) {
	tenantID := auth.GetTenantIdFromContext(ctx)
	if tenantID == 0 {
		return nil, v1.ErrorInvalidRequest("empty tenant id")
	}

	order, err := s.uc.Reject(ctx, tenantID, req.GetId())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, v1.ErrorNotFound("order not found")
		}
		return nil, v1.ErrorInvalidStatusTransition("%s", err.Error())
	}

	return &v1.RejectOrderReply{Order: replyOrder(order)}, nil
}

// --- 5.3: Order suggestions ---

func (s *OrdersService) GetOrderSuggestions(ctx context.Context, req *v1.GetOrderSuggestionsRequest) (*v1.GetOrderSuggestionsReply, error) {
	tenantID := auth.GetTenantIdFromContext(ctx)
	if tenantID == 0 {
		return nil, v1.ErrorInvalidRequest("empty tenant id")
	}

	// Placeholder: in production this would call warehouse gRPC to get low-stock items.
	// Returns empty list for now — integration with warehouse service is a separate story.
	return &v1.GetOrderSuggestionsReply{Items: []*v1.OrderSuggestion{}}, nil
}

// --- 5.4: List orders ---

func (s *OrdersService) ListOrders(ctx context.Context, req *v1.ListOrdersRequest) (*v1.ListOrdersReply, error) {
	tenantID := auth.GetTenantIdFromContext(ctx)
	if tenantID == 0 {
		return nil, v1.ErrorInvalidRequest("empty tenant id")
	}

	paginate := req.GetPaginate()
	if paginate == nil {
		paginate = &utils_v1.PaginateRequest{}
	}

	var status *enum.OrderStatus
	if req.GetFilterStatus() {
		s := enum.OrderStatus(req.GetStatus().String())
		status = &s
	}

	items, total, err := s.uc.List(ctx, tenantID, req.GetCounterpartyId(), status, paginate)
	if err != nil {
		return nil, err
	}

	orders := make([]*v1.Order, 0, len(items))
	for _, item := range items {
		orders = append(orders, replyOrder(item))
	}

	var fromID, toID *int64
	if len(items) > 0 {
		f := items[0].ID
		t := items[len(items)-1].ID
		fromID = &f
		toID = &t
	}

	return &v1.ListOrdersReply{
		Items: orders,
		Paginate: &utils_v1.PaginateReply{
			Total:  &total,
			FromId: fromID,
			ToId:   toID,
		},
	}, nil
}

// --- Helpers ---

func replyOrder(o *ent.Order) *v1.Order {
	resp := &v1.Order{
		Id:             o.ID,
		TenantId:       o.TenantID,
		CounterpartyId: o.CounterpartyID,
		Status:         protoStatus(o.Status),
		CreatedBy:      o.CreatedBy,
		CreatedAt:      o.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:      o.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if o.Edges.Items != nil {
		for _, item := range o.Edges.Items {
			resp.Items = append(resp.Items, replyOrderItem(item))
		}
	}

	return resp
}

func replyOrderItem(item *ent.OrderItem) *v1.OrderItem {
	return &v1.OrderItem{
		Id:        item.ID,
		OrderId:   item.OrderID,
		ProductId: item.ProductID,
		Quantity:  item.Quantity.String(),
		UnitPrice: item.UnitPrice.String(),
	}
}

func protoStatus(s enum.OrderStatus) v1.OrderStatus {
	switch s {
	case enum.Created:
		return v1.OrderStatus_CREATED
	case enum.Confirmed:
		return v1.OrderStatus_CONFIRMED
	case enum.Shipped:
		return v1.OrderStatus_SHIPPED
	case enum.Accepted:
		return v1.OrderStatus_ACCEPTED
	case enum.Rejected:
		return v1.OrderStatus_REJECTED
	default:
		return v1.OrderStatus_CREATED
	}
}

func parseDecimal(s string) decimal.Decimal {
	d, _ := decimal.NewFromString(s)
	return d
}
