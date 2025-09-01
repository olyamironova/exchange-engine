package grpc

import (
	"context"
	"github.com/olyamironova/exchange-engine/internal/core"
	"github.com/olyamironova/exchange-engine/internal/domain"
	"github.com/shopspring/decimal"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"

	_ "github.com/olyamironova/exchange-engine/internal/core"
	pb "github.com/olyamironova/exchange-engine/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GRPCServer struct {
	pb.UnimplementedExchangeServer
	Eng *core.Engine
}

func NewGRPCServer(eng *core.Engine) *GRPCServer {
	return &GRPCServer{Eng: eng}
}

func (s *GRPCServer) SubmitOrder(ctx context.Context, req *pb.SubmitOrderRequest) (*pb.SubmitOrderResponse, error) {
	if err := ValidateOrder(req); err != nil {
		return nil, err
	}

	price, err := decimal.NewFromString(req.Price)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid price: %v", err)
	}
	quantity, err := decimal.NewFromString(req.Quantity)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid quantity: %v", err)
	}

	o := &domain.Order{
		ClientID: req.ClientId,
		Symbol:   req.Symbol,
		Side:     domain.Side(req.Side),
		Type:     domain.OrderType(req.Type),
		Price:    price,
		Quantity: quantity,
	}

	trades, err := s.Eng.SubmitOrder(ctx, o)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "submit failed: %v", err)
	}

	pbTrades := make([]*pb.Trade, 0, len(trades))
	for _, t := range trades {
		pbTrades = append(pbTrades, &pb.Trade{
			Id:        t.ID,
			BuyOrder:  t.BuyOrder,
			SellOrder: t.SellOrder,
			Price:     t.Price.String(),
			Quantity:  t.Quantity.String(),
			Timestamp: TimeToProto(t.Timestamp),
		})
	}

	return &pb.SubmitOrderResponse{
		OrderId:   o.ID,
		Trades:    pbTrades,
		Remaining: o.Remaining.String(),
	}, nil
}

func (s *GRPCServer) ModifyOrder(ctx context.Context, req *pb.ModifyOrderRequest) (*pb.ModifyOrderResponse, error) {
	price, err := decimal.NewFromString(req.NewPrice)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid new_price: %v", err)
	}
	quantity, err := decimal.NewFromString(req.NewQuantity)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid new_quantity: %v", err)
	}

	if err := s.Eng.ModifyOrder(ctx, req.OrderId, req.ClientId, price, quantity); err != nil {
		return nil, status.Errorf(codes.Internal, "modify failed: %v", err)
	}
	return &pb.ModifyOrderResponse{
		OrderId:  req.OrderId,
		Modified: true,
		Message:  "",
	}, nil
}

func (s *GRPCServer) CancelOrder(ctx context.Context, req *pb.CancelOrderRequest) (*pb.CancelOrderResponse, error) {
	ok, err := s.Eng.CancelOrder(ctx, req.OrderId, req.ClientId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "cancel failed: %v", err)
	}
	return &pb.CancelOrderResponse{
		OrderId:   req.OrderId,
		Cancelled: ok,
		Message:   "",
	}, nil
}

func (s *GRPCServer) GetOrder(ctx context.Context, req *pb.GetOrderRequest) (*pb.GetOrderResponse, error) {
	order, err := s.Eng.GetOrder(ctx, req.OrderId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "order not found")
	}
	return &pb.GetOrderResponse{
		Order: convertOrderToPb(order),
	}, nil
}

func (s *GRPCServer) GetTradesForOrder(ctx context.Context, req *pb.GetTradesRequest) (*pb.GetTradesResponse, error) {
	trades, err := s.Eng.GetTradesForOrder(ctx, req.OrderId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get trades failed: %v", err)
	}
	pbTrades := make([]*pb.Trade, 0, len(trades))
	for _, t := range trades {
		pbTrades = append(pbTrades, &pb.Trade{
			Id:        t.ID,
			BuyOrder:  t.BuyOrder,
			SellOrder: t.SellOrder,
			Price:     t.Price.String(),
			Quantity:  t.Quantity.String(),
			Timestamp: TimeToProto(t.Timestamp),
		})
	}
	return &pb.GetTradesResponse{Trades: pbTrades}, nil
}

func (s *GRPCServer) GetOrderbook(ctx context.Context, req *pb.GetOrderbookRequest) (*pb.GetOrderbookResponse, error) {
	ob, err := s.Eng.GetOrderbook(ctx, req.Symbol)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "symbol not found")
	}
	copySnapshot := ob.DeepCopy()
	return &pb.GetOrderbookResponse{
		Bids:      convertOrdersToPb(copySnapshot.Bids),
		Asks:      convertOrdersToPb(copySnapshot.Asks),
		Timestamp: timestamppb.New(time.Now()),
	}, nil
}

func (s *GRPCServer) SnapshotOrderbook(ctx context.Context, req *pb.SnapshotRequest) (*pb.SnapshotResponse, error) {
	id, err := s.Eng.SnapshotOrderbook(ctx, req.Symbol)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "snapshot failed: %v", err)
	}
	return &pb.SnapshotResponse{
		SnapshotId: id,
		Message:    "snapshot created",
	}, nil
}

func (s *GRPCServer) RestoreOrderbook(ctx context.Context, req *pb.RestoreRequest) (*pb.RestoreResponse, error) {
	ok, err := s.Eng.RestoreOrderbook(ctx, req.SnapshotId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "restore failed: %v", err)
	}
	return &pb.RestoreResponse{
		Ok:      ok,
		Message: "restored successfully",
	}, nil
}

func convertOrderToPb(o *domain.Order) *pb.Order {
	return &pb.Order{
		Id:        o.ID,
		ClientId:  o.ClientID,
		Symbol:    o.Symbol,
		Side:      string(o.Side),
		Type:      string(o.Type),
		Price:     o.Price.String(),
		Quantity:  o.Quantity.String(),
		Remaining: o.Remaining.String(),
		CreatedAt: TimeToProto(o.CreatedAt),
	}
}

func convertOrdersToPb(in []domain.Order) []*pb.Order {
	out := make([]*pb.Order, 0, len(in))
	for _, o := range in {
		cpy := o
		out = append(out, convertOrderToPb(&cpy))
	}
	return out
}

func ValidateOrder(req *pb.SubmitOrderRequest) error {
	if req.Side != "BUY" && req.Side != "SELL" {
		return status.Errorf(codes.InvalidArgument, "invalid side: %s", req.Side)
	}
	if req.Type != "LIMIT" && req.Type != "MARKET" {
		return status.Errorf(codes.InvalidArgument, "invalid type: %s", req.Type)
	}
	return nil
}

func TimeToProto(t time.Time) *timestamppb.Timestamp { return timestamppb.New(t) }
