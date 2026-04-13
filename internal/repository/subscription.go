package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

type SubscriptionRepository struct {
	db *sqlx.DB
}

func NewSubscriptionRepository(db *sqlx.DB) *SubscriptionRepository {
	return &SubscriptionRepository{db: db}
}

func (r *SubscriptionRepository) CreateSubscription(ctx context.Context, id string, userID string, objectID string, objectType string) (string, error) {
	createdAt := time.Now()
	updatedAt := time.Now()
	
	query := `INSERT INTO subscriptions (id, user_id, object_id, object_type, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := r.db.ExecContext(ctx, query, id, userID, objectID, objectType, createdAt, updatedAt)
	if err != nil {
		return "", fmt.Errorf("failed to create subscription: %w", err)
	}
	return id, nil
}

func (r *SubscriptionRepository) GetSubscriptionByID(ctx context.Context, id string) (*domain.Subscription, error) {
	var sub domain.Subscription
	query := `SELECT id, user_id, object_id, object_type, created_at, updated_at FROM subscriptions WHERE id = $1`
	err := r.db.QueryRowContext(ctx, query, id).Scan(&sub.ID, &sub.UserID, &sub.ObjectID, &sub.ObjectType, &sub.CreatedAt, &sub.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get subscription by id: %w", err)
	}
	return &sub, nil
}

func (r *SubscriptionRepository) GetSubscriptionsByUserID(ctx context.Context, userID string) ([]domain.Subscription, error) {
	query := `SELECT id, user_id, object_id, object_type, created_at, updated_at FROM subscriptions WHERE user_id = $1`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscriptions by user id: %w", err)
	}
	defer rows.Close()

	var subs []domain.Subscription
	for rows.Next() {
		var sub domain.Subscription
		if err := rows.Scan(&sub.ID, &sub.UserID, &sub.ObjectID, &sub.ObjectType, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	return subs, nil
}

func (r *SubscriptionRepository) GetUserIDsByObject(ctx context.Context, objectID string, objectType string) ([]string, error) {
    query := `SELECT user_id FROM subscriptions WHERE object_id = $1 AND object_type = $2`
    rows, err := r.db.QueryContext(ctx, query, objectID, objectType)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var ids []string
    for rows.Next() {
        var id string
        if err := rows.Scan(&id); err != nil {
            return nil, err
        }
        ids = append(ids, id)
    }
    return ids, nil
}

func (r *SubscriptionRepository) UpdateSubscription(ctx context.Context, id string, userID string, objectID string, objectType string) error {
	updatedAt := time.Now()	
	query := `UPDATE subscriptions SET user_id = $1, object_id = $2, object_type = $3, updated_at = $4 WHERE id = $5`
	_, err := r.db.ExecContext(ctx, query, userID, objectID, objectType, updatedAt, id)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}
	return nil
}

func (r *SubscriptionRepository) DeleteSubscription(ctx context.Context, id string) error {
	query := `DELETE FROM subscriptions WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete subscription: %w", err)
	}
	return nil
}

