package server

import (
	"log"
	"net/http"
	"strconv"

	"github.com/robby-barton/stats-api/internal/common"

	"github.com/gin-gonic/gin"
)

func (s *Server) results(c *gin.Context) {
	year, week, ps, err := common.GetTimeframe(s.DB, 0, 0)
	if err != nil {
		log.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{})
		return
	}

	results, err := s.DB.Query("select * from team_week_result where year = $1 and week = $2 and postseason = $3", year, week, ps)
	if err != nil {
		log.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{})
	} else {
		c.JSON(http.StatusOK, results)
	}
}

func (s *Server) resultsForYear(c *gin.Context) {
	year, err := strconv.ParseInt(c.Param("year"), 10, 64)
	if err != nil {
		log.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{})
		return
	}

	_, week, ps, err := common.GetTimeframe(s.DB, year, 0)
	if err != nil {
		log.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{})
		return
	}

	results, err := s.DB.Query("select * from team_week_result where year = $1 and week = $2 and postseason = $3", year, week, ps)
	if err != nil {
		log.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{})
	} else {
		c.JSON(http.StatusOK, results)
	}
}

func (s *Server) resultsForWeek(c *gin.Context) {
	year, err := strconv.ParseInt(c.Param("year"), 10, 64)
	if err != nil {
		log.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{})
		return
	}

	week, err := strconv.ParseInt(c.Param("week"), 10, 64)
	if err != nil {
		log.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{})
		return
	}

	results, err := s.DB.Query("select * from team_week_result where year = $1 and week = $2 and postseason = $3", year, week, 0)
	if err != nil {
		log.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{})
	} else {
		c.JSON(http.StatusOK, results)
	}
}

func (s *Server) resultsRoutes() {
	s.APIServer.GET("/results", s.results)
	s.APIServer.GET("/results/:year", s.resultsForYear)
	s.APIServer.GET("/results/:year/:week", s.resultsForWeek)
}
