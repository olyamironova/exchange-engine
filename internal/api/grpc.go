package api

import (
	"context"
	_ "log"
	"time"

	"github.com/olyamironova/exchange-engine/internal/engine"
	pb "github.com/olyamironova/exchange-engine/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements pb.ExchangeServer
type Server struct {
	pb.UnimplementedExchangeServer
	Eng *engine.Engine
	// subs for streaming (simple in-memory pubsub)
	orderbookSubscribers *OrderbookPubSub
	tradeSubscribers     *TradePubSub
}

// NewServer creates a new gRPC server wrapper
func NewServer(eng *engine.Engine) *Server {
	return &Server{
		Eng:                  eng,
		orderbookSubscribers: NewOrderbookPubSub(),
		tradeSubscribers:     NewTradePubSub(),
	}
}

// SubmitOrder implements SubmitOrder RPC
func (s *Server) SubmitOrder(ctx context.Context, req *pb.SubmitOrderRequest) (*pb.SubmitOrderResponse, error) {
	o := &engine.Order{
		ClientID: req.ClientId,
		Symbol:   req.Symbol,
		Side:     engine.Side(req.Side),
		Type:     engine.OrderType(req.Type),
		Price:    req.Price,
		Quantity: req.Quantity,
	}
	// Dedup / rate-limit not implemented here (engine may handle)
	trades, err := s.Eng.SubmitOrder(ctx, o)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "submit failed: %v", err)
	}

	// Publish updates for streaming clients
	go s.orderbookSubscribers.PublishSnapshot(o.Symbol, s.Eng)
	for _, t := range trades {
		go s.tradeSubscribers.PublishTrade(o.Symbol, t)
	}

	pbTrades := make([]*pb.Trade, 0, len(trades))
	for _, tr := range trades {
		pbTrades = append(pbTrades, &pb.Trade{
			Id:            tr.ID,
			BuyOrder:      tr.BuyOrder,
			SellOrder:     tr.SellOrder,
			Price:         tr.Price,
			Quantity:      tr.Quantity,
			TimestampUnix: tr.Timestamp.Unix(),
		})
	}

	return &pb.SubmitOrderResponse{
		OrderId:   o.ID,
		Trades:    pbTrades,
		Remaining: o.Remaining,
	}, nil
}

// CancelOrder implements order cancellation
func (s *Server) CancelOrder(ctx context.Context, req *pb.CancelOrderRequest) (*pb.CancelOrderResponse, error) {
	ok, err := s.Eng.CancelOrder(ctx, req.OrderId, req.ClientId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "cancel error: %v", err)
	}
	if ok {
		// publish snapshot update
		order, _ := s.Eng.GetOrder(ctx, req.OrderId)
		if order != nil {
			go s.orderbookSubscribers.PublishSnapshot(order.Symbol, s.Eng)
		}
	}
	return &pb.CancelOrderResponse{
		OrderId:   req.OrderId,
		Cancelled: ok,
		Message:   "",
	}, nil
}

// GetOrderbook returns a snapshot
func (s *Server) GetOrderbook(ctx context.Context, req *pb.GetOrderbookRequest) (*pb.GetOrderbookResponse, error) {
	snap, err := s.Eng.GetOrderbook(req.Symbol)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "symbol not found")
	}
	bids := make([]*pb.Order, 0, len(snap.Bids))
	asks := make([]*pb.Order, 0, len(snap.Asks))
	for _, o := range snap.Bids {
		bids = append(bids, &pb.Order{
			Id:        o.ID,
			ClientId:  o.ClientID,
			Symbol:    o.Symbol,
			Side:      string(o.Side),
			Type:      string(o.Type),
			Price:     o.Price,
			Quantity:  o.Quantity,
			Remaining: o.Remaining,
			CreatedAt: o.CreatedAt.Unix(),
		})
	}
	for _, o := range snap.Asks {
		asks = append(asks, &pb.Order{
			Id:        o.ID,
			ClientId:  o.ClientID,
			Symbol:    o.Symbol,
			Side:      string(o.Side),
			Type:      string(o.Type),
			Price:     o.Price,
			Quantity:  o.Quantity,
			Remaining: o.Remaining,
			CreatedAt: o.CreatedAt.Unix(),
		})
	}
	return &pb.GetOrderbookResponse{
		Bids:      bids,
		Asks:      asks,
		Timestamp: time.Now().Unix(),
	}, nil
}

// StreamOrderbook server-side streaming implementation
func (s *Server) StreamOrderbook(req *pb.StreamOrderbookRequest, stream pb.Exchange_StreamOrderbookServer) error {
	ch := s.orderbookSubscribers.Subscribe(req.Symbol)
	defer s.orderbookSubscribers.Unsubscribe(req.Symbol, ch)

	// Send initial snapshot
	if snap, err := s.Eng.GetOrderbook(req.Symbol); err == nil {
		_ = stream.Send(&pb.OrderbookUpdate{
			Symbol:    req.Symbol,
			Bids:      convertOrdersToPb(snap.Bids),
			Asks:      convertOrdersToPb(snap.Asks),
			Timestamp: time.Now().Unix(),
		})
	}

	// Stream updates until client cancels
	for {
		select {
		case <-stream.Context().Done():
			return nil
		case <-ch:
			if snap, err := s.Eng.GetOrderbook(req.Symbol); err == nil {
				if err := stream.Send(&pb.OrderbookUpdate{
					Symbol:    req.Symbol,
					Bids:      convertOrdersToPb(snap.Bids),
					Asks:      convertOrdersToPb(snap.Asks),
					Timestamp: time.Now().Unix(),
				}); err != nil {
					return err
				}
			}
		}
	}
}

// StreamTrades: each message is a Trade pushed as executed
func (s *Server) StreamTrades(req *pb.StreamTradesRequest, stream pb.Exchange_StreamTradesServer) error {
	ch := s.tradeSubscribers.Subscribe(req.Symbol)
	defer s.tradeSubscribers.Unsubscribe(req.Symbol, ch)

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case tr := <-ch:
			if err := stream.Send(&pb.Trade{
				Id:            tr.ID,
				BuyOrder:      tr.BuyOrder,
				SellOrder:     tr.SellOrder,
				Price:         tr.Price,
				Quantity:      tr.Quantity,
				TimestampUnix: tr.Timestamp.Unix(),
			}); err != nil {
				return err
			}
		}
	}
}

func (s *Server) Health(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{Status: "ok"}, nil
}

func convertOrdersToPb(in []engine.Order) []*pb.Order {
	out := make([]*pb.Order, 0, len(in))
	for _, o := range in {
		cpy := o
		out = append(out, &pb.Order{
			Id:        cpy.ID,
			ClientId:  cpy.ClientID,
			Symbol:    cpy.Symbol,
			Side:      string(cpy.Side),
			Type:      string(cpy.Type),
			Price:     cpy.Price,
			Quantity:  cpy.Quantity,
			Remaining: cpy.Remaining,
			CreatedAt: cpy.CreatedAt.Unix(),
		})
	}
	return out
}

// Simple pubsub for orderbook updates
type OrderbookPubSub struct {
	subs map[string]map[chan struct{}]struct{}
}

func NewOrderbookPubSub() *OrderbookPubSub {
	return &OrderbookPubSub{
		subs: make(map[string]map[chan struct{}]struct{}),
	}
}

func (p *OrderbookPubSub) Subscribe(symbol string) chan struct{} {
	ch := make(chan struct{}, 1)
	if _, ok := p.subs[symbol]; !ok {
		p.subs[symbol] = make(map[chan struct{}]struct{})
	}
	p.subs[symbol][ch] = struct{}{}
	return ch
}

func (p *OrderbookPubSub) Unsubscribe(symbol string, ch chan struct{}) {
	if m, ok := p.subs[symbol]; ok {
		delete(m, ch)
		close(ch)
	}
}

func (p *OrderbookPubSub) PublishSnapshot(symbol string, eng *engine.Engine) {
	if m, ok := p.subs[symbol]; ok {
		for ch := range m {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	}
}

// Simple pubsub for trades. tradesChan carries *engine.Trade
type TradePubSub struct {
	subs map[string]map[chan *engine.Trade]struct{}
}

func NewTradePubSub() *TradePubSub {
	return &TradePubSub{
		subs: make(map[string]map[chan *engine.Trade]struct{}),
	}
}

func (p *TradePubSub) Subscribe(symbol string) chan *engine.Trade {
	ch := make(chan *engine.Trade, 10)
	if _, ok := p.subs[symbol]; !ok {
		p.subs[symbol] = make(map[chan *engine.Trade]struct{})
	}
	p.subs[symbol][ch] = struct{}{}
	return ch
}

func (p *TradePubSub) Unsubscribe(symbol string, ch chan *engine.Trade) {
	if m, ok := p.subs[symbol]; ok {
		delete(m, ch)
		close(ch)
	}
}

func (p *TradePubSub) PublishTrade(symbol string, t *engine.Trade) {
	// publish to symbol subscribers and to wildcard subscribers (key="")
	if m, ok := p.subs[symbol]; ok {
		for ch := range m {
			select {
			case ch <- t:
			default:
			}
		}
	}
	if m, ok := p.subs[""]; ok {
		for ch := range m {
			select {
			case ch <- t:
			default:
			}
		}
	}
}

func (s *Server) BatchSubmitOrders(ctx context.Context, req *pb.BatchSubmitOrdersRequest) (*pb.BatchSubmitOrdersResponse, error) {
	results := make([]*pb.SubmitOrderResponse, 0, len(req.Orders))
	for _, o := range req.Orders {
		res, err := s.SubmitOrder(ctx, o) // уже корректно, потому что SubmitOrder у Server
		if err != nil {
			return nil, err
		}
		results = append(results, res)
	}
	return &pb.BatchSubmitOrdersResponse{Results: results}, nil
}

func (s *Server) ModifyOrder(ctx context.Context, req *pb.ModifyOrderRequest) (*pb.ModifyOrderResponse, error) {
	ok, err := s.Eng.ModifyOrder(ctx, req.OrderId, req.ClientId, req.NewPrice, req.NewQuantity)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "modify failed: %v", err)
	}
	if ok {
		order, _ := s.Eng.GetOrder(ctx, req.OrderId)
		if order != nil {
			go s.orderbookSubscribers.PublishSnapshot(order.Symbol, s.Eng) // здесь используем orderbookSubscribers
		}
	}
	return &pb.ModifyOrderResponse{
		OrderId:  req.OrderId,
		Modified: ok,
		Message:  "",
	}, nil
}

func (s *Server) SnapshotOrderbook(ctx context.Context, req *pb.SnapshotRequest) (*pb.SnapshotResponse, error) {
	snapshotID, err := s.Eng.SnapshotOrderbook(req.Symbol)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "snapshot failed: %v", err)
	}
	return &pb.SnapshotResponse{
		SnapshotId: snapshotID,
		Message:    "snapshot created",
	}, nil
}

func (s *Server) RestoreOrderbook(ctx context.Context, req *pb.RestoreRequest) (*pb.RestoreResponse, error) {
	ok, err := s.Eng.RestoreOrderbook(req.SnapshotId)
	if err != nil {
		return &pb.RestoreResponse{Ok: false, Message: err.Error()}, nil
	}
	return &pb.RestoreResponse{Ok: ok, Message: "restored successfully"}, nil
}
