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
		Order("final_rank").
		Find(&teamResults).Error; err != nil {

		log.Println(err)
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
		Order("final_rank").
		Find(&teamResults).Error; err != nil {

		log.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{})
	} else {
		c.JSON(http.StatusOK, teamResults)
	}
}

func (s *Server) resultsForWeek(c *gin.Context) {
	var teamResults []database.TeamWeekResult

	if err := s.DB.Where("year = ? AND week = ?", c.Param("year"), c.Param("week")).
		Order("final_rank").
		Find(&teamResults).Error; err != nil {

		log.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{})
	} else {
		c.JSON(http.StatusOK, teamResults)
	}
}

func (s *Server) resultsFBS(c *gin.Context) {
	var teamResults []database.TeamWeekResult

	if err := s.DB.
		Where("year = (?) AND FBS = true", s.DB.Table("team_week_results").Select("MAX(year)")).
		Order("postseason desc").
		Order("week desc").
		Order("final_rank").
		Find(&teamResults).Error; err != nil {

		log.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{})
	} else {
		c.JSON(http.StatusOK, teamResults)
	}
}

func (s *Server) resultsForYearFBS(c *gin.Context) {
	var teamResults []database.TeamWeekResult

	if err := s.DB.
		Where("year = ? AND fbs = true", c.Param("year")).
		Order("postseason desc").
		Order("week desc").
		Order("final_rank").
		Find(&teamResults).Error; err != nil {

		log.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{})
	} else {
		c.JSON(http.StatusOK, teamResults)
	}
}

func (s *Server) resultsForWeekFBS(c *gin.Context) {
	var teamResults []database.TeamWeekResult

	if err := s.DB.
		Where("year = ? AND week = ? AND fbs = true", c.Param("year"), c.Param("week")).
		Order("final_rank").
		Find(&teamResults).Error; err != nil {

		log.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{})
	} else {
		c.JSON(http.StatusOK, teamResults)
	}
}

func (s *Server) resultsFCS(c *gin.Context) {
	var teamResults []database.TeamWeekResult

	if err := s.DB.
		Where("year = (?) AND fbs = false", s.DB.Table("team_week_results").Select("MAX(year)")).
		Order("postseason desc").
		Order("week desc").
		Order("final_rank").
		Find(&teamResults).Error; err != nil {

		log.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{})
	} else {
		c.JSON(http.StatusOK, teamResults)
	}
}

func (s *Server) resultsForYearFCS(c *gin.Context) {
	var teamResults []database.TeamWeekResult

	if err := s.DB.
		Where("year = ? AND fbs = false", c.Param("year")).
		Order("postseason desc").
		Order("week desc").
		Order("final_rank").
		Find(&teamResults).Error; err != nil {

		log.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{})
	} else {
		c.JSON(http.StatusOK, teamResults)
	}
}

func (s *Server) resultsForWeekFCS(c *gin.Context) {
	var teamResults []database.TeamWeekResult

	if err := s.DB.
		Where("year = ? AND week = ? AND fbs = false", c.Param("year"), c.Param("week")).
		Order("final_rank").
		Find(&teamResults).Error; err != nil {

		log.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{})
	} else {
		c.JSON(http.StatusOK, teamResults)
	}
}

func (s *Server) resultsRoutes() {
	s.APIServer.GET("/results", s.results)
	s.APIServer.GET("/results/:year", s.resultsForYear)
	s.APIServer.GET("/results/:year/:week", s.resultsForWeek)
	s.APIServer.GET("/results/fbs", s.resultsFBS)
	s.APIServer.GET("/results/fbs/:year", s.resultsForYearFBS)
	s.APIServer.GET("/results/fbs/:year/:week", s.resultsForWeekFBS)
	s.APIServer.GET("/results/fcs", s.resultsFCS)
	s.APIServer.GET("/results/fcs/:year", s.resultsForYearFCS)
	s.APIServer.GET("/results/fcs/:year/:week", s.resultsForWeekFCS)
}
