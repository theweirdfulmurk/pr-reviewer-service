package repository

import (
	"context"
	"time"
	"database/sql"

	"github.com/gocraft/dbr/v2"
	"go.uber.org/zap"

	"pr-reviewer-service/internal/types"
)

type Repository struct {
	sess   *dbr.Session
	logger *zap.Logger
}

func New(sess *dbr.Session, logger *zap.Logger) *Repository {
	return &Repository{sess: sess, logger: logger}
}

func (r *Repository) TeamExists(ctx context.Context, teamName string) (bool, error) {
	var count int
	err := r.sess.Select("COUNT(*)").
		From("teams").
		Where(dbr.Eq("team_name", teamName)).
		LoadOneContext(ctx, &count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) CreateTeam(ctx context.Context, team types.Team) error {
	tx, err := r.sess.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.RollbackUnlessCommitted()

	_, err = tx.InsertInto("teams").
		Columns("team_name").
		Values(team.TeamName).
		ExecContext(ctx)
	if err != nil {
		return err
	}

	for _, m := range team.Members {
		_, err = tx.InsertBySql(`
			INSERT INTO users (user_id, username, team_name, is_active)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (user_id) DO UPDATE SET username = ?, team_name = ?, is_active = ?`,
			m.UserID, m.Username, team.TeamName, m.IsActive,
			m.Username, team.TeamName, m.IsActive,
		).ExecContext(ctx)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *Repository) GetTeam(ctx context.Context, teamName string) (*types.Team, error) {
	var team struct {
		TeamName string `db:"team_name"`
	}
	err := r.sess.Select("team_name").
		From("teams").
		Where(dbr.Eq("team_name", teamName)).
		LoadOneContext(ctx, &team)
	if err != nil {
		if err == dbr.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}

	var members []types.TeamMember
	_, err = r.sess.Select("user_id", "username", "is_active").
		From("users").
		Where(dbr.Eq("team_name", teamName)).
		OrderBy("user_id").
		LoadContext(ctx, &members)
	if err != nil {
		return nil, err
	}

	if members == nil {
		members = []types.TeamMember{}
	}

	return &types.Team{
		TeamName: team.TeamName,
		Members:  members,
	}, nil
}

func (r *Repository) GetUser(ctx context.Context, userID string) (*types.User, error) {
	var user types.User
	err := r.sess.Select("user_id", "username", "team_name", "is_active").
		From("users").
		Where(dbr.Eq("user_id", userID)).
		LoadOneContext(ctx, &user)
	if err != nil {
		if err == dbr.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *Repository) SetUserActive(ctx context.Context, userID string, isActive bool) (*types.User, error) {
	result, err := r.sess.Update("users").
		Set("is_active", isActive).
		Where(dbr.Eq("user_id", userID)).
		ExecContext(ctx)
	if err != nil {
		return nil, err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return nil, nil
	}

	return r.GetUser(ctx, userID)
}

func (r *Repository) GetActiveTeamMembers(ctx context.Context, teamName string, excludeUserID string) ([]types.User, error) {
	var users []types.User
	_, err := r.sess.Select("user_id", "username", "team_name", "is_active").
		From("users").
		Where(dbr.And(
			dbr.Eq("team_name", teamName),
			dbr.Eq("is_active", true),
			dbr.Neq("user_id", excludeUserID),
		)).
		OrderBy("user_id").
		LoadContext(ctx, &users)
	if err != nil {
		return nil, err
	}
	return users, nil
}

func (r *Repository) PRExists(ctx context.Context, prID string) (bool, error) {
	var count int
	err := r.sess.Select("COUNT(*)").
		From("pull_requests").
		Where(dbr.Eq("pull_request_id", prID)).
		LoadOneContext(ctx, &count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) CreatePR(ctx context.Context, pr types.PullRequest) error {
	tx, err := r.sess.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.RollbackUnlessCommitted()

	_, err = tx.InsertInto("pull_requests").
		Columns("pull_request_id", "pull_request_name", "author_id", "status", "created_at").
		Values(pr.PullRequestID, pr.PullRequestName, pr.AuthorID, pr.Status, pr.CreatedAt).
		ExecContext(ctx)
	if err != nil {
		return err
	}

	for _, reviewerID := range pr.AssignedReviewers {
		_, err = tx.InsertInto("pr_reviewers").
			Columns("pull_request_id", "user_id").
			Values(pr.PullRequestID, reviewerID).
			ExecContext(ctx)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *Repository) GetPR(ctx context.Context, prID string) (*types.PullRequest, error) {
	var pr struct {
		PullRequestID   string         `db:"pull_request_id"`
		PullRequestName string         `db:"pull_request_name"`
		AuthorID        string         `db:"author_id"`
		Status          string         `db:"status"`
		CreatedAt       sql.NullTime   `db:"created_at"`
		MergedAt        sql.NullTime   `db:"merged_at"`
	}

	err := r.sess.Select("pull_request_id", "pull_request_name", "author_id", "status", "created_at", "merged_at").
		From("pull_requests").
		Where(dbr.Eq("pull_request_id", prID)).
		LoadOneContext(ctx, &pr)
	if err != nil {
		if err == dbr.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}

	var reviewers []string
	_, err = r.sess.Select("user_id").
		From("pr_reviewers").
		Where(dbr.Eq("pull_request_id", prID)).
		OrderBy("user_id").
		LoadContext(ctx, &reviewers)
	if err != nil {
		return nil, err
	}

	if reviewers == nil {
		reviewers = []string{}
	}

	result := &types.PullRequest{
		PullRequestID:     pr.PullRequestID,
		PullRequestName:   pr.PullRequestName,
		AuthorID:          pr.AuthorID,
		Status:            pr.Status,
		AssignedReviewers: reviewers,
	}

	if pr.CreatedAt.Valid {
		result.CreatedAt = &pr.CreatedAt.Time
	}
	if pr.MergedAt.Valid {
		result.MergedAt = &pr.MergedAt.Time
	}

	return result, nil
}

func (r *Repository) MergePR(ctx context.Context, prID string) (*types.PullRequest, error) {
	now := time.Now()
	_, err := r.sess.Update("pull_requests").
		Set("status", types.StatusMerged).
		Set("merged_at", now).
		Where(dbr.Eq("pull_request_id", prID)).
		ExecContext(ctx)
	if err != nil {
		return nil, err
	}

	return r.GetPR(ctx, prID)
}

func (r *Repository) ReassignReviewer(ctx context.Context, prID, oldUserID, newUserID string) error {
	result, err := r.sess.Update("pr_reviewers").
		Set("user_id", newUserID).
		Where(dbr.And(
			dbr.Eq("pull_request_id", prID),
			dbr.Eq("user_id", oldUserID),
		)).
		ExecContext(ctx)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) IsReviewerAssigned(ctx context.Context, prID, userID string) (bool, error) {
	var count int
	err := r.sess.Select("COUNT(*)").
		From("pr_reviewers").
		Where(dbr.And(
			dbr.Eq("pull_request_id", prID),
			dbr.Eq("user_id", userID),
		)).
		LoadOneContext(ctx, &count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) GetPRsByReviewer(ctx context.Context, userID string) ([]types.PullRequestShort, error) {
	var prs []types.PullRequestShort
	_, err := r.sess.Select("p.pull_request_id", "p.pull_request_name", "p.author_id", "p.status").
		From(dbr.I("pull_requests").As("p")).
		Join(dbr.I("pr_reviewers").As("r"), "p.pull_request_id = r.pull_request_id").
		Where(dbr.Eq("r.user_id", userID)).
		OrderBy("p.pull_request_id").
		LoadContext(ctx, &prs)
	if err != nil {
		return nil, err
	}
	return prs, nil
}

func (r *Repository) GetCurrentReviewers(ctx context.Context, prID string) ([]string, error) {
	var reviewers []string
	_, err := r.sess.Select("user_id").
		From("pr_reviewers").
		Where(dbr.Eq("pull_request_id", prID)).
		LoadContext(ctx, &reviewers)
	if err != nil {
		return nil, err
	}
	return reviewers, nil
}