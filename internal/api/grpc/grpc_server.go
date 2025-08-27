package grpc

import (
	"context"
	"github.com/olyamironova/exchange-engine/internal/api/http"
	"github.com/olyamironova/exchange-engine/internal/core"
	"github.com/olyamironova/exchange-engine/internal/domain"
	"time"

	_ "github.com/olyamironova/exchange-engine/internal/core"
	pb "github.com/olyamironova/exchange-engine/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GRPCServer struct {
	pb.UnimplementedExchangeServer // must embed for forward compatibility
	Eng                            *core.Engine
}

func NewGRPCServer(eng *core.Engine) *GRPCServer {
	return &GRPCServer{Eng: eng}
}

func (s *GRPCServer) SubmitOrder(ctx context.Context, req *pb.SubmitOrderRequest) (*pb.SubmitOrderResponse, error) {
	o := &domain.Order{
		ClientID: req.ClientId,
		Symbol:   req.Symbol,
		Side:     domain.Side(req.Side),
		Type:     domain.OrderType(req.Type),
		Price:    req.Price,
		Quantity: req.Quantity,
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
			Price:     t.Price,
			Quantity:  t.Quantity,
			Timestamp: http.TimeToProto(t.Timestamp),
		})
	}

	return &pb.SubmitOrderResponse{
		OrderId:   o.ID,
		Trades:    pbTrades,
		Remaining: o.Remaining,
	}, nil
}

func (s *GRPCServer) ModifyOrder(ctx context.Context, req *pb.ModifyOrderRequest) (*pb.ModifyOrderResponse, error) {
	ok, err := s.Eng.ModifyOrder(ctx, req.OrderId, req.ClientId, req.NewPrice, req.NewQuantity)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "modify failed: %v", err)
	}
	return &pb.ModifyOrderResponse{
		OrderId:  req.OrderId,
		Modified: ok,
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
	trades, _ := s.Eng.GetTradesForOrder(ctx, req.OrderId)
	pbTrades := make([]*pb.Trade, 0, len(trades))
	for _, t := range trades {
		pbTrades = append(pbTrades, &pb.Trade{
			Id:        t.ID,
			BuyOrder:  t.BuyOrder,
			SellOrder: t.SellOrder,
			Price:     t.Price,
			Quantity:  t.Quantity,
			Timestamp: http.TimeToProto(t.Timestamp),
		})
	}
	return &pb.GetTradesResponse{Trades: pbTrades}, nil
}

func (s *GRPCServer) GetOrderbook(ctx context.Context, req *pb.GetOrderbookRequest) (*pb.GetOrderbookResponse, error) {
	ob, err := s.Eng.GetOrderbook(ctx, req.Symbol)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "symbol not found")
	}
	return &pb.GetOrderbookResponse{
		Bids:      convertOrdersToPb(ob.Bids),
		Asks:      convertOrdersToPb(ob.Asks),
		Timestamp: time.Now().Unix(),
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
		Price:     o.Price,
		Quantity:  o.Quantity,
		Remaining: o.Remaining,
		CreatedAt: http.TimeToProto(o.CreatedAt),
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
