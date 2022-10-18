package server

import (
	"github.com/robby-barton/stats-api/internal/config"
	"github.com/robby-barton/stats-api/internal/database"

	"github.com/gin-gonic/gin"
	limiter "github.com/ulule/limiter/v3"
	"github.com/ulule/limiter/v3/drivers/store/memory"
	mgin "github.com/ulule/limiter/v3/drivers/middleware/gin"
)

type Server struct {
	APIServer *gin.Engine
	DB        *database.DB
	CFG       *config.Config
}

func NewServer() (*Server, error) {
	apiServer := gin.Default()

	middleware, err := createLimiterMiddleware()
	if err != nil {
		return nil, err
	}
	apiServer.Use(middleware)

	cfg := config.SetupConfig()

	db, err := database.NewDatabase(cfg.DBParams)
	if err != nil {
		return nil, err
	}

	server := &Server{
		APIServer: apiServer,
		DB:        db,
		CFG:       cfg,
	}

	server.addRoutes()

	return server, nil
}

func createLimiterMiddleware() (gin.HandlerFunc, error){
	rate, err := limiter.NewRateFromFormatted("5-S")
	if err != nil {
		return nil, err
	}

	store := memory.NewStore()
	
	return mgin.NewMiddleware(limiter.New(store, rate)), nil
}

func (s *Server) addRoutes() {
	s.teamsRoutes()
	s.resultsRoutes()
}
