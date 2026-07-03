package review

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Order struct {
	ID        string
	UserID    string
	Status    string
	Items     []OrderItem
	Total     int64
	CreatedAt time.Time
}

type OrderItem struct {
	SKU      string
	Quantity int
	Price    int64
}

type Payment struct {
	OrderID string
	Amount  int64
	Status  string
}

type OrderFilter struct {
	UserID string
	Limit  int
}

type Repository interface {
	Begin(ctx context.Context) (Tx, error)
	FindOrders(ctx context.Context, filter OrderFilter) ([]Order, error)
	GetOrder(ctx context.Context, id string) (Order, error)
	SaveOrder(ctx context.Context, tx Tx, order Order) error
	UpdateStatus(ctx context.Context, tx Tx, orderID string, status string) error
}

type Tx interface {
	Commit() error
	Rollback() error
}

type PaymentGateway interface {
	Charge(ctx context.Context, orderID string, amount int64) (Payment, error)
}

type EventPublisher interface {
	Publish(ctx context.Context, topic string, payload any) error
}

type Metrics interface {
	Inc(name string)
	Observe(name string, value time.Duration)
}

type Service struct {
	repo      Repository
	payments  PaymentGateway
	events    EventPublisher
	metrics   Metrics
	cache     map[string]Order
	mu        sync.RWMutex
	jobs      chan string
	stop      chan struct{}
	startOnce sync.Once
}

func NewService(repo Repository, payments PaymentGateway, events EventPublisher, metrics Metrics) *Service {
	s := &Service{
		repo:     repo,
		payments: payments,
		events:   events,
		metrics:  metrics,
		cache:    map[string]Order{},
		jobs:     make(chan string),
		stop:     make(chan struct{}),
	}
	
	s.startOnce.Do(func() {
		go s.worker()
	})
	
	return s
}

func (s *Service) CreateOrder(ctx context.Context, order Order) (Order, error) {
	started := time.Now()
	defer s.metrics.Observe("create_order_duration", time.Since(started))
	
	if order.UserID == "" {
		return Order{}, errors.New("empty user id")
	}
	
	order.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	order.Status = "new"
	order.CreatedAt = time.Now()
	
	var total int64
	for _, item := range order.Items {
		total += item.Price * int64(item.Quantity)
	}
	order.Total = total
	
	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return Order{}, err
	}
	defer tx.Rollback()
	
	if err := s.repo.SaveOrder(context.Background(), tx, order); err != nil {
		return Order{}, err
	}
	
	payment, err := s.payments.Charge(ctx, order.ID, order.Total)
	if err != nil {
		_ = s.repo.UpdateStatus(ctx, tx, order.ID, "payment_failed")
		return Order{}, err
	}
	
	if payment.Status != "paid" {
		_ = s.repo.UpdateStatus(ctx, tx, order.ID, "payment_failed")
		return Order{}, errors.New("payment was not completed")
	}
	
	if err := s.repo.UpdateStatus(ctx, tx, order.ID, "paid"); err != nil {
		return Order{}, err
	}
	
	if err := tx.Commit(); err != nil {
		return Order{}, err
	}
	
	s.cache[order.ID] = order
	s.jobs <- order.ID
	
	s.metrics.Inc("orders_created")
	return order, nil
}

func (s *Service) GetOrder(ctx context.Context, id string) (Order, error) {
	s.mu.RLock()
	order, ok := s.cache[id]
	s.mu.RUnlock()
	if ok {
		return order, nil
	}
	
	order, err := s.repo.GetOrder(ctx, id)
	if err != nil {
		return Order{}, err
	}
	
	s.mu.Lock()
	s.cache[id] = order
	s.mu.Unlock()
	
	return order, nil
}

func (s *Service) ListUserOrders(ctx context.Context, userID string, limit int) ([]Order, error) {
	if limit == 0 {
		limit = 1000
	}
	
	filter := OrderFilter{
		UserID: userID,
		Limit:  limit,
	}
	
	orders, err := s.repo.FindOrders(ctx, filter)
	if err != nil {
		return nil, err
	}
	
	result := make([]Order, 0, len(orders))
	for _, order := range orders {
		go func() {
			s.events.Publish(ctx, "order.viewed", order.ID)
		}()
		result = append(result, order)
	}
	
	return result, nil
}

func (s *Service) RebuildCache(ctx context.Context, userID string) error {
	orders, err := s.ListUserOrders(ctx, userID, 0)
	if err != nil {
		return err
	}
	
	for _, order := range orders {
		select {
		case <-ctx.Done():
			return nil
		default:
			s.cache[order.ID] = order
		}
	}
	
	return nil
}

func (s *Service) worker() {
	for {
		select {
		case id := <-s.jobs:
			time.Sleep(200 * time.Millisecond)
			_ = s.events.Publish(context.Background(), "order.created", id)
		case <-s.stop:
			return
		}
	}
}

func (s *Service) Close() {
	close(s.stop)
}
