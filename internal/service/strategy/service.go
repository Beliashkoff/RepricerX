// Package strategy реализует бизнес-логику управления стратегиями ценообразования.
package strategy

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrStrategyNotFound      = errors.New("strategy not found")
	ErrInvalidStrategyType   = errors.New("invalid strategy type")
	ErrInvalidStrategyParams = errors.New("invalid strategy params")
	ErrInvalidConstraints    = errors.New("invalid constraints")
	ErrProductNotFound       = errors.New("product not found")
)

type CreateInput struct {
	Name           string
	Type           string
	Params         json.RawMessage
	Constraints    json.RawMessage
	FallbackPolicy string
	Priority       int
	Enabled        bool
}

type UpdatePatch struct {
	Name           *string
	Type           *string
	Params         json.RawMessage
	Constraints    json.RawMessage
	FallbackPolicy *string
	Priority       *int
	Enabled        *bool
}

type Service struct {
	strategies  repository.StrategiesRepository
	assignments repository.StrategyAssignmentsRepository
}

func New(strategies repository.StrategiesRepository, assignments repository.StrategyAssignmentsRepository) *Service {
	return &Service{strategies: strategies, assignments: assignments}
}

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]*domain.Strategy, error) {
	return s.strategies.ListByUser(ctx, userID)
}

func (s *Service) Get(ctx context.Context, userID, id uuid.UUID) (*domain.Strategy, error) {
	st, err := s.strategies.GetByIDForUser(ctx, userID, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrStrategyNotFound
		}
		return nil, err
	}
	return st, nil
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, in CreateInput) (*domain.Strategy, error) {
	if err := validateType(in.Type); err != nil {
		return nil, err
	}
	if err := validateFallbackPolicy(in.FallbackPolicy); err != nil {
		return nil, err
	}
	if err := validateParams(in.Type, in.Params); err != nil {
		return nil, err
	}
	if err := validateConstraints(in.Constraints); err != nil {
		return nil, err
	}

	params, _ := in.Params.MarshalJSON()
	var constraints []byte
	if len(in.Constraints) > 0 {
		constraints, _ = in.Constraints.MarshalJSON()
	} else {
		constraints = []byte("{}")
	}

	return s.strategies.Create(ctx, userID, repository.StrategyCreateInput{
		Name:           in.Name,
		Type:           in.Type,
		Params:         params,
		Constraints:    constraints,
		FallbackPolicy: in.FallbackPolicy,
		Priority:       in.Priority,
		Enabled:        in.Enabled,
	})
}

func (s *Service) Update(ctx context.Context, userID, id uuid.UUID, patch UpdatePatch) (*domain.Strategy, error) {
	// Validate type if changing.
	if patch.Type != nil {
		if err := validateType(*patch.Type); err != nil {
			return nil, err
		}
	}
	if patch.FallbackPolicy != nil {
		if err := validateFallbackPolicy(*patch.FallbackPolicy); err != nil {
			return nil, err
		}
	}

	// Resolve effective type for params validation.
	effectiveType := ""
	if patch.Type != nil {
		effectiveType = *patch.Type
	} else if len(patch.Params) > 0 {
		// Need current type to validate params.
		cur, err := s.strategies.GetByIDForUser(ctx, userID, id)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, ErrStrategyNotFound
			}
			return nil, err
		}
		effectiveType = cur.Type
	}

	if len(patch.Params) > 0 && effectiveType != "" {
		if err := validateParams(effectiveType, patch.Params); err != nil {
			return nil, err
		}
	}
	if len(patch.Constraints) > 0 {
		if err := validateConstraints(patch.Constraints); err != nil {
			return nil, err
		}
	}

	inp := repository.StrategyUpdateInput{
		Name:           patch.Name,
		Type:           patch.Type,
		FallbackPolicy: patch.FallbackPolicy,
		Priority:       patch.Priority,
		Enabled:        patch.Enabled,
	}
	if len(patch.Params) > 0 {
		inp.Params, _ = patch.Params.MarshalJSON()
	}
	if len(patch.Constraints) > 0 {
		inp.Constraints, _ = patch.Constraints.MarshalJSON()
	}

	st, err := s.strategies.Update(ctx, userID, id, inp)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrStrategyNotFound
		}
		return nil, err
	}
	return st, nil
}

func (s *Service) Delete(ctx context.Context, userID, id uuid.UUID) error {
	err := s.strategies.Delete(ctx, userID, id)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrStrategyNotFound
	}
	return err
}

func (s *Service) AssignedProductIDs(ctx context.Context, userID, strategyID uuid.UUID) ([]uuid.UUID, error) {
	if _, err := s.Get(ctx, userID, strategyID); err != nil {
		return nil, err
	}
	return s.assignments.ListProductIDsByStrategy(ctx, userID, strategyID)
}

func (s *Service) AssignToProducts(ctx context.Context, userID, strategyID uuid.UUID, productIDs []uuid.UUID) error {
	if _, err := s.Get(ctx, userID, strategyID); err != nil {
		return err
	}
	if len(productIDs) == 0 {
		return nil
	}
	return s.assignments.AssignToProducts(ctx, userID, strategyID, productIDs)
}

func (s *Service) UnassignFromProducts(ctx context.Context, userID, strategyID uuid.UUID, productIDs []uuid.UUID) error {
	if _, err := s.Get(ctx, userID, strategyID); err != nil {
		return err
	}
	if len(productIDs) == 0 {
		return nil
	}
	return s.assignments.UnassignFromProducts(ctx, userID, strategyID, productIDs)
}

func (s *Service) AssignedCount(ctx context.Context, strategyID uuid.UUID) int {
	n, _ := s.strategies.CountAssignments(ctx, strategyID)
	return n
}
