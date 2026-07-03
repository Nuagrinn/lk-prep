package transactionreview

import (
	"context"
	"errors"
	"time"
)

type Booking struct {
	ID         string
	UserID     string
	RoomID     string
	Status     string
	StartsAt   time.Time
	EndsAt     time.Time
	AccessCode string
}

type AuditEntry struct {
	BookingID string
	Action    string
	CreatedAt time.Time
}

type CalendarEvent struct {
	BookingID string
	UserID    string
	RoomID    string
	StartsAt  time.Time
	EndsAt    time.Time
}

type BookingConfirmed struct {
	BookingID  string
	UserID     string
	RoomID     string
	AccessCode string
}

type BookingRepository interface {
	Begin(ctx context.Context) (Tx, error)
	GetBooking(ctx context.Context, tx Tx, id string) (Booking, error)
	MarkBookingConfirmed(ctx context.Context, tx Tx, id string, accessCode string) error
	MarkBookingFailed(ctx context.Context, tx Tx, id string, reason string) error
	SaveAuditEntry(ctx context.Context, tx Tx, entry AuditEntry) error
}

type Tx interface {
	Commit() error
	Rollback() error
}

type AccessClient interface {
	CreateAccessCode(ctx context.Context, roomID string, userID string, from time.Time, to time.Time) (string, error)
}

type CalendarClient interface {
	CreateEvent(ctx context.Context, event CalendarEvent) error
}

type EventPublisher interface {
	Publish(ctx context.Context, topic string, payload any) error
}

type Cache interface {
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
}

type BookingService struct {
	repo     BookingRepository
	access   AccessClient
	calendar CalendarClient
	events   EventPublisher
	cache    Cache
}

func NewBookingService(repo BookingRepository, access AccessClient, calendar CalendarClient, events EventPublisher, cache Cache) *BookingService {
	return &BookingService{
		repo:     repo,
		access:   access,
		calendar: calendar,
		events:   events,
		cache:    cache,
	}
}

func (s *BookingService) ConfirmBooking(ctx context.Context, bookingID string) (Booking, error) {
	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return Booking{}, err
	}
	
	//роллбэк без обработки ошибки
	defer tx.Rollback()
	
	booking, err := s.repo.GetBooking(ctx, tx, bookingID)
	if err != nil {
		return Booking{}, err
	}
	
	if booking.Status != "pending" {
		return Booking{}, errors.New("booking is not pending")
	}
	
	//сервисный внешний сайд эффект
	accessCode, err := s.access.CreateAccessCode(ctx, booking.RoomID, booking.UserID, booking.StartsAt, booking.EndsAt)
	if err != nil {
		// ошибка может не записаться, если делаем роллбэк
		_ = s.repo.MarkBookingFailed(ctx, tx, booking.ID, "access_code_failed")
		return Booking{}, err
	}
	
	if err := s.repo.MarkBookingConfirmed(ctx, tx, booking.ID, accessCode); err != nil {
		return Booking{}, err
	}
	
	booking.Status = "confirmed"
	booking.AccessCode = accessCode
	
	// меняем состояние кэша, даже если транзакция откатится
	if err := s.cache.Set(ctx, "booking:"+booking.ID, booking, 15*time.Minute); err != nil {
		return Booking{}, err
	}
	
	//очередной сайд-эффект вызов внутри транзакции, нужно вынести в отдельный метод
	if err := s.calendar.CreateEvent(ctx, CalendarEvent{
		BookingID: booking.ID,
		UserID:    booking.UserID,
		RoomID:    booking.RoomID,
		StartsAt:  booking.StartsAt,
		EndsAt:    booking.EndsAt,
	}); err != nil {
		//ошибка также же может откатиться
		_ = s.repo.MarkBookingFailed(ctx, tx, booking.ID, "calendar_event_failed")
		return Booking{}, err
	}
	
	// аудит лог может потеряться
	if err := s.repo.SaveAuditEntry(ctx, tx, AuditEntry{
		BookingID: booking.ID,
		Action:    "booking_confirmed",
		CreatedAt: time.Now(),
	}); err != nil {
		return Booking{}, err
	}
	
	//публикация события до коммита, может отправить сообщение о несуществующем событии
	if err := s.events.Publish(ctx, "booking.confirmed", BookingConfirmed{
		BookingID:  booking.ID,
		UserID:     booking.UserID,
		RoomID:     booking.RoomID,
		AccessCode: accessCode,
	}); err != nil {
		return Booking{}, err
	}
	
	if err := tx.Commit(); err != nil {
		return Booking{}, err
	}
	
	return booking, nil
}
