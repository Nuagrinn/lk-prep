package contextpractice

import (
	"context"
	"errors"
	"time"
)

type Document struct {
	ID        string
	UserID    string
	Status    string
	CreatedAt time.Time
}

type CheckResult struct {
	DocumentID string
	Approved   bool
	Reason     string
}

type DocumentRepository interface {
	GetDocument(ctx context.Context, id string) (Document, error)
	MarkChecking(ctx context.Context, id string) error
	SaveCheckResult(ctx context.Context, result CheckResult) error
	MarkFailed(ctx context.Context, id string, reason string) error
}

type RiskClient interface {
	CheckDocument(ctx context.Context, document Document) (CheckResult, error)
}

type EventPublisher interface {
	Publish(ctx context.Context, topic string, payload any) error
}

type DocumentService struct {
	repo   DocumentRepository
	risk   RiskClient
	events EventPublisher
}

func NewDocumentService(repo DocumentRepository, risk RiskClient, events EventPublisher) *DocumentService {
	return &DocumentService{
		repo:   repo,
		risk:   risk,
		events: events,
	}
}

func (s *DocumentService) StartDocumentCheck(ctx context.Context, documentID string) error {
	if documentID == "" {
		return errors.New("empty document id")
	}
	
	// передаем background ctx, из-за чего запрос в бд не получит сигнал об отмене
	document, err := s.repo.GetDocument(context.Background(), documentID)
	if err != nil {
		return err
	}
	
	if document.Status != "uploaded" {
		return errors.New("document is not uploaded")
	}
	
	// возможна утечка контекста, т.к. есть ветки кода без отмены
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	
	// создаем контекст слишком низки, сервис не будет управлять этим контекстом
	if err := s.repo.MarkChecking(checkCtx, document.ID); err != nil {
		return err
	}
	
	// одну горутину засунули и репо и скорее всего публикацию в брокер
	go func() {
		//s.risk.CheckDocument(ctx, document) использует request context внутри goroutine.
		//После возврата StartDocumentCheck HTTP request может завершиться, и ctx отменится.
		//Проверка документа может оборваться сразу после успешного ответа клиенту.
		result, err := s.risk.CheckDocument(ctx, document)
		if err != nil {
			//игнорируем ошибку, передаем пустой контекст
			_ = s.repo.MarkFailed(context.Background(), document.ID, err.Error())
			return
		}
		
		//игнорируем ошибку, передаем пустой контекст
		if err := s.repo.SaveCheckResult(context.Background(), result); err != nil {
			return
		}
		
		//игнорируем ошибку
		_ = s.events.Publish(ctx, "document.checked", result)
	}()
	
	//select с default почти бесполезен.
	//Он проверяет ctx.Done() один раз и сразу идет дальше.
	//Если ctx отменится через миллисекунду после этого, метод уже вернет nil.
	select {
	case <-ctx.Done():
		cancel()
		//при отмене возвращается nil. Это ошибка. Надо возвращать ctx.Err():
		return nil
	default:
	}
	
	//cancel() вызывается вручную в конце, но лучше defer cancel() сразу после создания.
	//Сейчас при ошибке на MarkChecking cancel не вызовется.
	cancel()
	return nil
}
