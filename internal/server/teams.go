package server

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/robby-barton/stats-api/internal/database"
)

func (s *Server) teams(c *gin.Context) {
	var teams []database.TeamName

	result := s.DB.
		Find(&teams)
	if result.Error != nil {
		log.Println(result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{})
	} else {
		c.JSON(http.StatusOK, teams)
	}
}

func (s *Server) teamById(c *gin.Context) {
	teamId := c.Param("id")

	var team database.TeamName

	result := s.DB.
		Where("team_id = ?", teamId).
		Find(&team)
	if result.Error != nil {
		log.Println(result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{})
	} else {
		c.JSON(http.StatusOK, team)
	}
}

func (s *Server) resultsByTeam(c *gin.Context) {
	teamId := c.Param("team")

	var teamResults []database.TeamWeekResult

	result := s.DB.
		Where("team_id = ?", teamId).
		Order("year desc").
		Order("postseason desc").
		Order("week desc").
		Find(&teamResults)
	if result.Error != nil {
		log.Println(result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{})
	} else {
		c.JSON(http.StatusOK, teamResults)
	}
}

func (s *Server) teamsRoutes() {
	s.APIServer.GET("/teams", s.teams)
	s.APIServer.GET("/team/:id", s.teamById)
	s.APIServer.GET("/team/:id/results", s.resultsByTeam)
}
