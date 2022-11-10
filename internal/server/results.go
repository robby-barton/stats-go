package server

import (
	"log"
	"net/http"

	"github.com/robby-barton/stats-go/internal/database"

	"github.com/gin-gonic/gin"
)

func (s *Server) results(c *gin.Context) {
	var teamResults []database.TeamWeekResult

	if err := s.DB.Where("year = (?)", s.DB.Table("team_week_results").Select("MAX(year)")).
		Order("postseason desc").
		Order("week desc").
		Order("final_rank desc").
		Find(&teamResults).Error; err != nil {

		log.Println(result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{})
	} else {
		c.JSON(http.StatusOK, teamResults)
	}
}

func (s *Server) resultsForYear(c *gin.Context) {
	var teamResults []database.TeamWeekResult

	if err := s.DB.Where("year = ?", c.Param("year")).
		Order("postseason desc").
		Order("week desc").
		Order("final_rank desc").
		Find(&teamResults).Error; err != nil {

		log.Println(result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{})
	} else {
		c.JSON(http.StatusOK, teamResults)
	}
}

func (s *Server) resultsForWeek(c *gin.Context) {
	var teamResults []database.TeamWeekResult

	if err := s.DB.Where("year = ? AND week = ?", c.Param("year"), c.Param("week")).
		Order("final_rank desc").
		Find(&teamResults).Error; err != nil {

		log.Println(result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{})
	} else {
		c.JSON(http.StatusOK, teamResults)
	}
}

func (s *Server) resultsRoutes() {
	s.APIServer.GET("/results", s.results)
	s.APIServer.GET("/results/:year", s.resultsForYear)
	s.APIServer.GET("/results/:year/:week", s.resultsForWeek)
}
