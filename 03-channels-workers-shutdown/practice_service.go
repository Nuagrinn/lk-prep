package channelpractice

import (
	"context"
	"errors"
	"time"
)

type Report struct {
	ID        string
	UserID    string
	Email     string
	Status    string
	CreatedAt time.Time
}

type ReportJob struct {
	ReportID string
	Email    string
}

type ReportRepository interface {
	GetReport(ctx context.Context, id string) (Report, error)
	MarkQueued(ctx context.Context, id string) error
	MarkSent(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string, reason string) error
}

type ReportSender interface {
	SendReport(ctx context.Context, reportID string, email string) error
}

type Metrics interface {
	Inc(name string)
	Observe(name string, value time.Duration)
}

type ReportService struct {
	repo    ReportRepository
	sender  ReportSender
	metrics Metrics
	jobs    chan ReportJob
	stop    chan struct{}
}

func NewReportService(repo ReportRepository, sender ReportSender, metrics Metrics) *ReportService {
	s := &ReportService{
		repo:    repo,
		sender:  sender,
		metrics: metrics,
		//канал небуферизованный, горутина может засыпать на нем
		jobs: make(chan ReportJob),
		stop: make(chan struct{}),
	}
	
	//не описан lifecycle, возвращается инстанс сервиса, при этом параллельно запускается воркер, живущий своей жизнью
	//нет waitGroup
	go s.worker()
	
	return s
}

func (s *ReportService) QueueReport(ctx context.Context, reportID string) error {
	if reportID == "" {
		return errors.New("empty report id")
	}
	
	report, err := s.repo.GetReport(ctx, reportID)
	if err != nil {
		return err
	}
	
	if report.Status != "ready" {
		return errors.New("report is not ready")
	}
	
	if err := s.repo.MarkQueued(ctx, report.ID); err != nil {
		return err
	}
	
	//если будет запуск этого метода из другой горутины, то он повиснет
	//можем записать в закрытиый канал, нет обработки этого сценария
	s.jobs <- ReportJob{
		ReportID: report.ID,
		Email:    report.Email,
	}
	
	s.metrics.Inc("reports_queued")
	return nil
}

//“может повиснуть, если нет задач” тоже не ошибка сама по себе. Worker и должен ждать работу.
func (s *ReportService) worker() {
	//busy loop
	for {
		//вот тут наоборот: это не busy loop. select без default блокируется и не крутит CPU.
		//default как раз мог бы сделать busy loop. То, что worker ждёт задачу или stop-сигнал, нормально.
		select {
		//получение из закрытого канала без ok
		case job := <-s.jobs:
			started := time.Now()
			
			// worker использует context.Background(). Shutdown не отменит SendReport, MarkFailed, MarkSent;
			//есть timeout только на отправку, а repo-вызовы вообще без lifecycle context.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := s.sender.SendReport(ctx, job.ReportID, job.Email)
			cancel()
			
			if err != nil {
				//игнорируем ошибку
				_ = s.repo.MarkFailed(context.Background(), job.ReportID, err.Error())
				continue
			}
			
			//игнорируем ошибку
			_ = s.repo.MarkSent(context.Background(), job.ReportID)
			//лучше использовать Ticker (?)
			//Ticker тут не нужен. time.Since(started) для duration-метрики как раз нормальный вариант.
			//Ticker нужен для периодических задач, а не для измерения длительности.
			s.metrics.Observe("report_send_duration", time.Since(started))
		case <-s.stop:
			return
		}
	}
}

func (s *ReportService) Close() {
	// неидемпотентное закрытие канала
	//нет sync.Once
	close(s.stop)
}
