package service

import (
	"context"
	"math/rand"
	"time"

	"go.uber.org/zap"

	"pr-reviewer-service/internal/repository"
	"pr-reviewer-service/internal/types"
)

type Service struct {
	repo   *repository.Repository
	rng    *rand.Rand
	logger *zap.Logger
}

func New(repo *repository.Repository, logger *zap.Logger) *Service {
	return &Service{
		repo:   repo,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
		logger: logger,
	}
}

type ServiceError struct {
	Code    types.ErrorCode
	Message string
}

func (e *ServiceError) Error() string {
	return e.Message
}

func (s *Service) CreateTeam(ctx context.Context, req types.CreateTeamRequest) (*types.Team, error) {
	s.logger.Debug("creating team", zap.String("team_name", req.TeamName))

	exists, err := s.repo.TeamExists(ctx, req.TeamName)
	if err != nil {
		s.logger.Error("failed to check team existence", zap.Error(err))
		return nil, err
	}
	if exists {
		return nil, &ServiceError{Code: types.ErrTeamExists, Message: "team_name already exists"}
	}

	team := types.Team{
		TeamName: req.TeamName,
		Members:  req.Members,
	}

	if err := s.repo.CreateTeam(ctx, team); err != nil {
		s.logger.Error("failed to create team", zap.Error(err))
		return nil, err
	}

	s.logger.Info("team created", zap.String("team_name", req.TeamName), zap.Int("members_count", len(req.Members)))
	return &team, nil
}

func (s *Service) GetTeam(ctx context.Context, teamName string) (*types.Team, error) {
	team, err := s.repo.GetTeam(ctx, teamName)
	if err != nil {
		s.logger.Error("failed to get team", zap.Error(err))
		return nil, err
	}
	if team == nil {
		return nil, &ServiceError{Code: types.ErrNotFound, Message: "team not found"}
	}
	return team, nil
}

func (s *Service) SetUserActive(ctx context.Context, userID string, isActive bool) (*types.User, error) {
	s.logger.Debug("setting user active status", zap.String("user_id", userID), zap.Bool("is_active", isActive))

	user, err := s.repo.SetUserActive(ctx, userID, isActive)
	if err != nil {
		s.logger.Error("failed to set user active", zap.Error(err))
		return nil, err
	}
	if user == nil {
		return nil, &ServiceError{Code: types.ErrNotFound, Message: "user not found"}
	}

	s.logger.Info("user active status updated", zap.String("user_id", userID), zap.Bool("is_active", isActive))
	return user, nil
}

func (s *Service) CreatePR(ctx context.Context, req types.CreatePRRequest) (*types.PullRequest, error) {
	s.logger.Debug("creating PR", zap.String("pr_id", req.PullRequestID), zap.String("author_id", req.AuthorID))

	exists, err := s.repo.PRExists(ctx, req.PullRequestID)
	if err != nil {
		s.logger.Error("failed to check PR existence", zap.Error(err))
		return nil, err
	}
	if exists {
		return nil, &ServiceError{Code: types.ErrPRExists, Message: "PR id already exists"}
	}

	author, err := s.repo.GetUser(ctx, req.AuthorID)
	if err != nil {
		s.logger.Error("failed to get author", zap.Error(err))
		return nil, err
	}
	if author == nil {
		return nil, &ServiceError{Code: types.ErrNotFound, Message: "author not found"}
	}

	candidates, err := s.repo.GetActiveTeamMembers(ctx, author.TeamName, author.UserID)
	if err != nil {
		s.logger.Error("failed to get team members", zap.Error(err))
		return nil, err
	}

	reviewers := s.selectRandomReviewers(candidates, 2, nil)

	now := time.Now()
	pr := types.PullRequest{
		PullRequestID:     req.PullRequestID,
		PullRequestName:   req.PullRequestName,
		AuthorID:          req.AuthorID,
		Status:            types.StatusOpen,
		AssignedReviewers: reviewers,
		CreatedAt:         &now,
	}

	if err := s.repo.CreatePR(ctx, pr); err != nil {
		s.logger.Error("failed to create PR", zap.Error(err))
		return nil, err
	}

	s.logger.Info("PR created",
		zap.String("pr_id", req.PullRequestID),
		zap.String("author_id", req.AuthorID),
		zap.Strings("reviewers", reviewers))
	return &pr, nil
}

func (s *Service) MergePR(ctx context.Context, prID string) (*types.PullRequest, error) {
	s.logger.Debug("merging PR", zap.String("pr_id", prID))

	pr, err := s.repo.GetPR(ctx, prID)
	if err != nil {
		s.logger.Error("failed to get PR", zap.Error(err))
		return nil, err
	}
	if pr == nil {
		return nil, &ServiceError{Code: types.ErrNotFound, Message: "PR not found"}
	}

	if pr.Status == types.StatusMerged {
		s.logger.Debug("PR already merged", zap.String("pr_id", prID))
		return pr, nil
	}

	result, err := s.repo.MergePR(ctx, prID)
	if err != nil {
		s.logger.Error("failed to merge PR", zap.Error(err))
		return nil, err
	}

	s.logger.Info("PR merged", zap.String("pr_id", prID))
	return result, nil
}

func (s *Service) ReassignReviewer(ctx context.Context, prID, oldUserID string) (*types.PullRequest, string, error) {
	s.logger.Debug("reassigning reviewer", zap.String("pr_id", prID), zap.String("old_user_id", oldUserID))

	pr, err := s.repo.GetPR(ctx, prID)
	if err != nil {
		s.logger.Error("failed to get PR", zap.Error(err))
		return nil, "", err
	}
	if pr == nil {
		return nil, "", &ServiceError{Code: types.ErrNotFound, Message: "PR not found"}
	}

	if pr.Status == types.StatusMerged {
		return nil, "", &ServiceError{Code: types.ErrPRMerged, Message: "cannot reassign on merged PR"}
	}

	assigned, err := s.repo.IsReviewerAssigned(ctx, prID, oldUserID)
	if err != nil {
		s.logger.Error("failed to check reviewer assignment", zap.Error(err))
		return nil, "", err
	}
	if !assigned {
		return nil, "", &ServiceError{Code: types.ErrNotAssigned, Message: "reviewer is not assigned to this PR"}
	}

	oldUser, err := s.repo.GetUser(ctx, oldUserID)
	if err != nil {
		s.logger.Error("failed to get old reviewer", zap.Error(err))
		return nil, "", err
	}
	if oldUser == nil {
		return nil, "", &ServiceError{Code: types.ErrNotFound, Message: "user not found"}
	}

	currentReviewers, err := s.repo.GetCurrentReviewers(ctx, prID)
	if err != nil {
		s.logger.Error("failed to get current reviewers", zap.Error(err))
		return nil, "", err
	}

	candidates, err := s.repo.GetActiveTeamMembers(ctx, oldUser.TeamName, pr.AuthorID)
	if err != nil {
		s.logger.Error("failed to get team members", zap.Error(err))
		return nil, "", err
	}

	exclude := make(map[string]bool)
	for _, r := range currentReviewers {
		exclude[r] = true
	}

	var filtered []types.User
	for _, c := range candidates {
		if !exclude[c.UserID] {
			filtered = append(filtered, c)
		}
	}

	if len(filtered) == 0 {
		return nil, "", &ServiceError{Code: types.ErrNoCandidate, Message: "no active replacement candidate in team"}
	}

	newReviewer := filtered[s.rng.Intn(len(filtered))]

	if err := s.repo.ReassignReviewer(ctx, prID, oldUserID, newReviewer.UserID); err != nil {
		s.logger.Error("failed to reassign reviewer", zap.Error(err))
		return nil, "", err
	}

	pr, err = s.repo.GetPR(ctx, prID)
	if err != nil {
		s.logger.Error("failed to get updated PR", zap.Error(err))
		return nil, "", err
	}

	s.logger.Info("reviewer reassigned",
		zap.String("pr_id", prID),
		zap.String("old_reviewer", oldUserID),
		zap.String("new_reviewer", newReviewer.UserID))
	return pr, newReviewer.UserID, nil
}

func (s *Service) GetUserReviews(ctx context.Context, userID string) (*types.GetReviewResponse, error) {
	prs, err := s.repo.GetPRsByReviewer(ctx, userID)
	if err != nil {
		s.logger.Error("failed to get user reviews", zap.Error(err))
		return nil, err
	}

	if prs == nil {
		prs = []types.PullRequestShort{}
	}

	return &types.GetReviewResponse{
		UserID:       userID,
		PullRequests: prs,
	}, nil
}

func (s *Service) selectRandomReviewers(candidates []types.User, max int, exclude map[string]bool) []string {
	var available []types.User
	for _, c := range candidates {
		if exclude == nil || !exclude[c.UserID] {
			available = append(available, c)
		}
	}

	if len(available) == 0 {
		return []string{}
	}

	s.rng.Shuffle(len(available), func(i, j int) {
		available[i], available[j] = available[j], available[i]
	})

	count := max
	if len(available) < count {
		count = len(available)
	}

	result := make([]string, count)
	for i := 0; i < count; i++ {
		result[i] = available[i].UserID
	}

	return result
}