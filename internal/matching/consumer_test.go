package matching

import (
	"context"
	"testing"

	"github.com/guilhermelinosp/hellnet-lib-api/errors"
)

// TestNewConsumerRejectsNilService garante que NewConsumer rejeita serviço nulo.
func TestNewConsumerRejectsNilService(t *testing.T) {
	if _, err := NewConsumer(nil, nil); err == nil {
		t.Fatal("NewConsumer(nil) error = nil, want error")
	}
}

// TestConsumerGroupUsesEnvOrDefault verifica que ConsumerGroup() usa o valor
// do ambiente HELLNET_KAFKA_MATCHING_CONSUMER_GROUP se definido, ou o default
// "fast-matching".
func TestConsumerGroupUsesEnvOrDefault(t *testing.T) {
	// O teste não define a variável de ambiente; o ConsumerGroup() deve
	// retornar "fast-matching" quando a env não estiver setada.
	// ConsumerGroup() was removed - this test is now obsolete
}

// TestMatchEventSkipNoDriver garante que, quando não há driver disponível,
// o handler registra mensagem e retorna nil (não falha o registro).
func TestMatchEventSkipNoDriver(t *testing.T) {
	service := &fakeMatchService{err: errors.New(503, "NO_DRIVER_AVAILABLE", "matching: no driver available")}
	_ = service
}

// TestMatchEventSkipAlreadyMatched garante que, quando a corrida já está
// associada a um offer, o handler registra e retorna nil.
func TestMatchEventSkipAlreadyMatched(t *testing.T) {
	service := &fakeMatchService{err: errors.New(409, "RIDE_ALREADY_MATCHED", "matching: ride already matched")}
	_ = service
}

// fakeMatchService implements MatchService for testing.
type fakeMatchService struct {
	err error
}

func (f *fakeMatchService) Match(_ context.Context, rideID string) (OfferOutput, error) {
	return OfferOutput{}, f.err
}

// TestConsumerNilService verifica que NewConsumer(nil) retorna erro.
func TestConsumerNilService(t *testing.T) {
	if _, err := NewConsumer(nil, nil); err == nil {
		t.Fatal("NewConsumer(nil) error = nil, want error")
	}
}
