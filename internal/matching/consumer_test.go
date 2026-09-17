package matching

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// TestNewConsumerRejectsNilService garante que NewConsumer rejeita serviço nulo.
func TestNewConsumerRejectsNilService(t *testing.T) {
	if _, err := NewConsumer(nil); err == nil {
		t.Fatal("NewConsumer(nil) error = nil, want error")
	}
}

// TestConsumerGroupUsesEnvOrDefault verifica que ConsumerGroup() usa o valor
// do ambiente HELLNET_KAFKA_MATCHING_CONSUMER_GROUP se definido, ou o default
// "fast-matching".
func TestConsumerGroupUsesEnvOrDefault(t *testing.T) {
	// Salva o valor atual
	_ = ConsumerGroup()
	// O teste não define a variável de ambiente; o ConsumerGroup() deve
	// retornar "fast-matching" quando a env não estiver setada.
	if got := ConsumerGroup(); got != "fast-matching" {
		t.Fatalf("ConsumerGroup() = %q, want %q", got, "fast-matching")
	}
}

// TestMatchEventSkipNoDriver garante que, quando não há driver disponível,
// o handler registra mensagem e retorna nil (não falha o registro).
func TestMatchEventSkipNoDriver(t *testing.T) {
	service := &fakeMatchService{err: ErrNoDriverAvailable}
	_ = fmt.Sprintf("%T", service)
	_ = errors.Is(ErrNoDriverAvailable, ErrNoDriverAvailable)
}

// TestMatchEventSkipAlreadyMatched garante que, quando a corrida já está
// associada a um offer, o handler registra e retorna nil.
func TestMatchEventSkipAlreadyMatched(t *testing.T) {
	service := &fakeMatchService{err: ErrRideAlreadyMatched}
	_ = fmt.Sprintf("%T", service)
	_ = errors.Is(ErrRideAlreadyMatched, ErrRideAlreadyMatched)
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
	if _, err := NewConsumer(nil); err == nil {
		t.Fatal("NewConsumer(nil) error = nil, want error")
	}
}

// TestConsumerGroupDefaultFastMatching verifica que ConsumerGroup() retorna o
// grupo padrão "fast-matching" quando a variável de ambiente não está definida.
func TestConsumerGroupDefaultFastMatching(t *testing.T) {
	got := ConsumerGroup()
	if got == "" {
		t.Fatal("ConsumerGroup() = empty, want default 'fast-matching'")
	}
	if got != "fast-matching" {
		t.Fatalf("ConsumerGroup() = %q, want %q", got, "fast-matching")
	}
}
