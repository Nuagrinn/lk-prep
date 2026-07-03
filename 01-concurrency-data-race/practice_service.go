package concurrencyreview

import (
	"context"
	"errors"
	"sync"
	"time"
)

type Product struct {
	ID        string
	Name      string
	Price     int64
	Available bool
	UpdatedAt time.Time
}

type ProductRepository interface {
	GetProduct(ctx context.Context, id string) (Product, error)
	SaveProduct(ctx context.Context, product Product) error
}

type ProductService struct {
	repo      ProductRepository
	cache     map[string]Product
	mu        sync.RWMutex
	lastSeen  time.Time
	viewCount int
}

func NewProductService(repo ProductRepository) *ProductService {
	return &ProductService{
		repo:  repo,
		cache: make(map[string]Product),
	}
}

func (s *ProductService) GetProduct(ctx context.Context, id string) (Product, error) {
	s.mu.RLock()
	product, ok := s.cache[id]
	s.mu.RUnlock()
	if ok {
		// каунтер инкрементится неатомарно, как и lastSeen
		s.viewCount++
		s.lastSeen = time.Now()
		return product, nil
	}
	
	product, err := s.repo.GetProduct(ctx, id)
	if err != nil {
		return Product{}, err
	}
	
	s.mu.Lock()
	s.cache[id] = product
	s.mu.Unlock()
	
	// каунтер инкрементится неатомарно, как и lastSeen
	s.viewCount++
	s.lastSeen = time.Now()
	
	return product, nil
}

func (s *ProductService) UpdateProduct(ctx context.Context, product Product) error {
	if product.ID == "" {
		return errors.New("empty product id")
	}
	
	product.UpdatedAt = time.Now()
	
	if err := s.repo.SaveProduct(ctx, product); err != nil {
		return err
	}
	
	//неатомарные операции над кэшэм и lastSeen
	s.cache[product.ID] = product
	s.lastSeen = product.UpdatedAt
	
	return nil
}

func (s *ProductService) Warmup(ctx context.Context, ids []string) error {
	var wg sync.WaitGroup
	
	for _, id := range ids {
		wg.Add(1)
		go func(productID string) {
			defer wg.Done()
			
			product, err := s.repo.GetProduct(ctx, productID)
			if err != nil {
				return
			}
			
			//неатомарный апдейт кэша
			s.cache[product.ID] = product
		}(id)
	}
	
	wg.Wait()
	return nil
}

func (s *ProductService) Stats() (int, time.Time) {
	// возвращаем переменные без RWMutex, т.е. надо уточнять, будет ли этот метод в месте использования еще защищаться мьютексами
	return s.viewCount, s.lastSeen
}
