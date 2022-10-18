package server

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) teams(c *gin.Context) {
	teams, err := s.DB.Query("select * from team_name")
	if err != nil {
		log.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{})
	} else {
		c.JSON(http.StatusOK, teams)
	}
}

func (s *Server) teamById(c *gin.Context) {
	teamId := c.Param("id")
	team, err := s.DB.Query("select * from team_name where team_id = $1", teamId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{})
	} else {
		c.JSON(http.StatusOK, team)
	}
}

func (s *Server) resultsByTeam(c *gin.Context) {
	teamId := c.Param("team")
	results, err := s.DB.Query("select * from team_week_result where team_id = $1 order by year, week, postseason desc", teamId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{})
	} else {
		c.JSON(http.StatusOK, results)
	}
}

func (s *Server) teamsRoutes() {
	s.APIServer.GET("/teams", s.teams)
	s.APIServer.GET("/team/:id", s.teamById)
	s.APIServer.GET("/team/:id/results", s.resultsByTeam)
}
