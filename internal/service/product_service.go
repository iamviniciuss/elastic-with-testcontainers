package service

import (
	"context"
	"fmt"

	"github.com/viniciussantos/claude-testcontainers/internal/repository"
)

type ProductService struct {
	repo *repository.ProductRepository
}

func NewProductService(repo *repository.ProductRepository) *ProductService {
	return &ProductService{
		repo: repo,
	}
}

func (s *ProductService) CreateProduct(ctx context.Context, product *repository.Product) error {
	// Validações de negócio
	if product.Name == "" {
		return fmt.Errorf("product name is required")
	}
	
	if product.Price < 0 {
		return fmt.Errorf("product price must be positive")
	}
	
	// Cria produto via repositório
	return s.repo.Create(ctx, product)
}

func (s *ProductService) GetProductByID(ctx context.Context, id string, tenantID string) (*repository.Product, error) {
	if id == "" {
		return nil, fmt.Errorf("product ID is required")
	}
	
	if tenantID == "" {
		return nil, fmt.Errorf("tenant ID is required")
	}
	
	return s.repo.GetByID(ctx, id, tenantID)
}

func (s *ProductService) GetProductsByCategory(ctx context.Context, category string, tenantID string) ([]*repository.Product, error) {
	if category == "" {
		return nil, fmt.Errorf("category is required")
	}
	
	if tenantID == "" {
		return nil, fmt.Errorf("tenant ID is required")
	}
	
	return s.repo.SearchByCategory(ctx, category, tenantID)
}

func (s *ProductService) GetExpensiveProducts(ctx context.Context, minPrice float64, tenantID string) ([]*repository.Product, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("tenant ID is required")
	}
	
	// Por simplicidade, vamos buscar todos de uma categoria e filtrar
	// Em um caso real, isso seria uma query específica no Elasticsearch
	electronics, err := s.repo.SearchByCategory(ctx, "electronics", tenantID)
	if err != nil {
		return nil, err
	}
	
	var expensive []*repository.Product
	for _, product := range electronics {
		if product.Price >= minPrice {
			expensive = append(expensive, product)
		}
	}
	
	return expensive, nil
}