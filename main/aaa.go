package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Profile struct {
	UserID string
	Name   string
}

type ProfileClient interface {
	FetchProfile(ctx context.Context, userID string) (Profile, error)
}

type Service struct {
	client ProfileClient
	cache  map[string]Profile
	rmu    sync.RWMutex
	wg     sync.WaitGroup
}

func NewService(client ProfileClient) *Service {
	return &Service{
		client: client,
		cache:  make(map[string]Profile),
	}
}

func (s *Service) GetProfiles(ctx context.Context, userIDs []string) ([]Profile, error) {
	resultCh := make(chan Profile)
	
	for _, userID := range userIDs {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			//конкуретное чтение из кэша
			s.rmu.RLock()
			profile, ok := s.cache[userID]
			s.rmu.RUnlock()
			
			if ok {
				resultCh <- profile
				return
			}
			
			//внешний вызов внутри горутины может, но это вроде ок
			profile, err := s.client.FetchProfile(ctx, userID)
			if err != nil {
				return
			}
			
			// конкурентная запись в мап из разых горутин, вызовет панику
			s.rmu.Lock()
			s.cache[userID] = profile
			s.rmu.Unlock()
			//никто не закрывает канал
			resultCh <- profile
		}()
	}
	
	go func() {
		s.wg.Wait()
		close(resultCh)
	}()
	
	var result []Profile
	ticker := time.NewTicker(5 * time.Second)
	for range userIDs {
		select {
		case profile := <-resultCh:
			result = append(result, profile)
		//лучше использовать тикер, т.к. 	time.After будет создавать новые таймеры
		//и один из запросов может вернуть оишбку по всему методу
		case <-ticker.C:
			return nil, fmt.Errorf("timeout")
		}
	}
	
	return result, nil
}
