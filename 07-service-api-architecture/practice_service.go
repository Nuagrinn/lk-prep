package architecturepractice

import (
	"context"
	"errors"
	"net/http"
	"time"
)

type User struct {
	ID        string
	Email     string
	Name      string
	Status    string
	LastSeen  time.Time
	CreatedAt time.Time
}

type OrderSummary struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Total     int64  `json:"total"`
	CreatedAt string `json:"created_at"`
}

type UserProfileResponse struct {
	ID        string         `json:"id"`
	Email     string         `json:"email"`
	Name      string         `json:"name"`
	Status    string         `json:"status"`
	Orders    []OrderSummary `json:"orders,omitempty"`
	LastSeen  string         `json:"last_seen"`
	CreatedAt string         `json:"created_at"`
}

type UserRepository interface {
	GetUser(ctx context.Context, id string) (User, error)
	ListUserOrders(ctx context.Context, userID string, limit int) ([]OrderSummary, error)
	UpdateLastSeen(ctx context.Context, userID string, at time.Time) error
	SaveUser(ctx context.Context, user User) error
	DeleteUser(ctx context.Context, id string) error
	FindUsers(ctx context.Context, query string, limit int) ([]User, error)
}

type EventBus interface {
	Publish(ctx context.Context, topic string, payload any) error
}

type Mailer interface {
	SendProfileViewedEmail(ctx context.Context, email string, userName string) error
}

type Cache interface {
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
}

type ProfileService struct {
	users  UserRepository
	events EventBus
	mailer Mailer
	cache  Cache
}

// параметры можно занести в отдельную config-структуру, проще будет расширять
func NewProfileService(users UserRepository, events EventBus, mailer Mailer, cache Cache) *ProfileService {
	return &ProfileService{
		users:  users,
		events: events,
		mailer: mailer,
		cache:  cache,
	}
}

/*
- слишком много параметров
- флаги, через которые регулируется сценарий метода
- возвращает респонс-модель прямо из сервиса
- название метода обманывает, скрывает сайд-эффекты
*/

func (s *ProfileService) GetProfile(ctx context.Context, r *http.Request, userID string, includeOrders bool, markViewed bool, sendEmail bool) (UserProfileResponse, error) {
	if r == nil {
		return UserProfileResponse{}, errors.New("empty request")
	}
	
	//просочилась транспортная логика
	if r.Header.Get("X-Internal-User") == "" {
		return UserProfileResponse{}, errors.New("unauthorized")
	}
	
	user, err := s.users.GetUser(ctx, userID)
	if err != nil {
		//неинформативные ошибки
		return UserProfileResponse{}, err
	}
	
	profile := UserProfileResponse{
		ID:        user.ID,
		Email:     user.Email,
		Name:      user.Name,
		Status:    user.Status,
		LastSeen:  user.LastSeen.Format(time.RFC3339),
		CreatedAt: user.CreatedAt.Format(time.RFC3339),
	}
	
	/*
	   includeOrders bool делает один метод несколькими сценариями;
	   limit = 100 захардкожен;
	   ListUserOrders лежит в UserRepository, хотя это уже зона заказов/read model;
	   repository возвращает []OrderSummary с json-тегами, то есть наружная response-модель протекла в storage/service boundary.
	*/
	if includeOrders {
		orders, err := s.users.ListUserOrders(ctx, userID, 100)
		if err != nil {
			return UserProfileResponse{}, err
		}
		profile.Orders = orders
	}
	
	//незалогированные ошибки
	// сайд-эффекты: публикация событий, изменение в методе query, внешние вызовы, изменение кэша
	
	if markViewed {
		_ = s.users.UpdateLastSeen(ctx, userID, time.Now())
		//UpdateLastSeen упал, а Publish прошёл, система сообщает о просмотре, которого не записала.
		_ = s.events.Publish(ctx, "user.profile_viewed", userID)
	}
	
	if sendEmail {
		// goroutine: go s.mailer.Send... без lifecycle, без обработки ошибки, с request context.
		go s.mailer.SendProfileViewedEmail(ctx, user.Email, user.Name)
	}
	
	/*
		profile собирается до UpdateLastSeen, а кешируется после.
		Если markViewed == true, в кеше может оказаться профиль со старым LastSeen, хотя в базе уже записали новый.
	*/
	_ = s.cache.Set(ctx, "profile:"+userID, profile, 5*time.Minute)
	
	return profile, nil
}

// метод переименовать, название более соотвествующее, конкретное, параметры передвать через команд-структуру
