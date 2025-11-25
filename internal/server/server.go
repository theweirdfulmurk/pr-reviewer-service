package server

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"

	"pr-reviewer-service/internal/service"
	"pr-reviewer-service/internal/types"
)

type Server struct {
	app     *fiber.App
	service *service.Service
	logger  *zap.Logger
}

func New(svc *service.Service, log *zap.Logger) *Server {
	app := fiber.New(fiber.Config{
		ErrorHandler: customErrorHandler,
	})

	s := &Server{
		app:     app,
		service: svc,
		logger:  log,
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

func (s *Server) setupMiddleware() {
	s.app.Use(recover.New())
	s.app.Use(logger.New())
}

func (s *Server) setupRoutes() {
	s.app.Post("/team/add", s.handleCreateTeam)
	s.app.Get("/team/get", s.handleGetTeam)
	s.app.Post("/users/setIsActive", s.handleSetIsActive)
	s.app.Get("/users/getReview", s.handleGetReview)
	s.app.Post("/pullRequest/create", s.handleCreatePR)
	s.app.Post("/pullRequest/merge", s.handleMergePR)
	s.app.Post("/pullRequest/reassign", s.handleReassign)
	s.app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})
}

func (s *Server) handleCreateTeam(c *fiber.Ctx) error {
	var req types.CreateTeamRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(types.NewAPIError(types.ErrNotFound, "invalid request body"))
	}
	if req.TeamName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(types.NewAPIError(types.ErrNotFound, "team_name is required"))
	}
	for _, m := range req.Members {
		if m.UserID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(types.NewAPIError(types.ErrNotFound, "member.user_id is required"))
		}
		if m.Username == "" {
			return c.Status(fiber.StatusBadRequest).JSON(types.NewAPIError(types.ErrNotFound, "member.username is required"))
		}
	}

	team, err := s.service.CreateTeam(c.Context(), req)
	if err != nil {
		return handleServiceError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(types.CreateTeamResponse{Team: *team})
}

func (s *Server) handleGetTeam(c *fiber.Ctx) error {
	teamName := c.Query("team_name")
	if teamName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(types.NewAPIError(types.ErrNotFound, "team_name is required"))
	}

	team, err := s.service.GetTeam(c.Context(), teamName)
	if err != nil {
		return handleServiceError(c, err)
	}

	return c.JSON(team)
}

func (s *Server) handleSetIsActive(c *fiber.Ctx) error {
	var req types.SetIsActiveRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(types.NewAPIError(types.ErrNotFound, "invalid request body"))
	}
	if req.UserID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(types.NewAPIError(types.ErrNotFound, "user_id is required"))
	}

	user, err := s.service.SetUserActive(c.Context(), req.UserID, req.IsActive)
	if err != nil {
		return handleServiceError(c, err)
	}

	return c.JSON(types.SetIsActiveResponse{User: *user})
}

func (s *Server) handleGetReview(c *fiber.Ctx) error {
	userID := c.Query("user_id")
	if userID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(types.NewAPIError(types.ErrNotFound, "user_id is required"))
	}

	resp, err := s.service.GetUserReviews(c.Context(), userID)
	if err != nil {
		return handleServiceError(c, err)
	}

	return c.JSON(resp)
}

func (s *Server) handleCreatePR(c *fiber.Ctx) error {
	var req types.CreatePRRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(types.NewAPIError(types.ErrNotFound, "invalid request body"))
	}
	if req.PullRequestID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(types.NewAPIError(types.ErrNotFound, "pull_request_id is required"))
	}
	if req.PullRequestName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(types.NewAPIError(types.ErrNotFound, "pull_request_name is required"))
	}
	if req.AuthorID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(types.NewAPIError(types.ErrNotFound, "author_id is required"))
	}

	pr, err := s.service.CreatePR(c.Context(), req)
	if err != nil {
		return handleServiceError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(types.CreatePRResponse{PR: *pr})
}

func (s *Server) handleMergePR(c *fiber.Ctx) error {
	var req types.MergePRRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(types.NewAPIError(types.ErrNotFound, "invalid request body"))
	}
	if req.PullRequestID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(types.NewAPIError(types.ErrNotFound, "pull_request_id is required"))
	}

	pr, err := s.service.MergePR(c.Context(), req.PullRequestID)
	if err != nil {
		return handleServiceError(c, err)
	}

	return c.JSON(types.MergePRResponse{PR: *pr})
}

func (s *Server) handleReassign(c *fiber.Ctx) error {
	var req types.ReassignRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(types.NewAPIError(types.ErrNotFound, "invalid request body"))
	}
	if req.PullRequestID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(types.NewAPIError(types.ErrNotFound, "pull_request_id is required"))
	}
	if req.OldUserID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(types.NewAPIError(types.ErrNotFound, "old_user_id is required"))
	}

	pr, replacedBy, err := s.service.ReassignReviewer(c.Context(), req.PullRequestID, req.OldUserID)
	if err != nil {
		return handleServiceError(c, err)
	}

	return c.JSON(types.ReassignResponse{PR: *pr, ReplacedBy: replacedBy})
}

func handleServiceError(c *fiber.Ctx, err error) error {
	var svcErr *service.ServiceError
	if errors.As(err, &svcErr) {
		status := fiber.StatusInternalServerError
		switch svcErr.Code {
		case types.ErrNotFound:
			status = fiber.StatusNotFound
		case types.ErrTeamExists:
			status = fiber.StatusBadRequest
		case types.ErrPRExists, types.ErrPRMerged, types.ErrNotAssigned, types.ErrNoCandidate:
			status = fiber.StatusConflict
		}
		return c.Status(status).JSON(types.NewAPIError(svcErr.Code, svcErr.Message))
	}
	return c.Status(fiber.StatusInternalServerError).JSON(types.NewAPIError(types.ErrNotFound, "internal server error"))
}

func customErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	var e *fiber.Error
	if errors.As(err, &e) {
		code = e.Code
	}
	return c.Status(code).JSON(types.NewAPIError(types.ErrNotFound, err.Error()))
}

func (s *Server) Listen(addr string) error {
	return s.app.Listen(addr)
}

func (s *Server) Shutdown() error {
	return s.app.Shutdown()
}
