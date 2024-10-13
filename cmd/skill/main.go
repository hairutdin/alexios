package main

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/hairutdin/alexios/internal/logger"
	"github.com/hairutdin/alexios/internal/store/pg"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
)

func main() {
	parseFlags()

	if err := run(); err != nil {
		panic(err)
	}
}

func gzipMiddleware(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ow := w

		acceptEncoding := r.Header.Get("Accept-Encoding")
		supportsGzip := strings.Contains(acceptEncoding, "gzip")
		if supportsGzip {
			cw := newCompressWriter(w)
			ow = cw
			defer cw.Close()
		}

		contentEncoding := r.Header.Get("Content-Encoding")
		sendsGzip := strings.Contains(contentEncoding, "gzip")
		if sendsGzip {
			cr, err := newCompressReader(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			r.Body = cr
			defer cr.Close()
		}

		h.ServeHTTP(ow, r)
	}
}

func run() error {
	if err := logger.Initialize(flagLogLevel); err != nil {
		return err
	}

	conn, err := sql.Open("pgx", flagDatabaseURI)
	if err != nil {
		return err
	}

	appInstance := newApp(pg.NewStore(conn))

	logger.Log.Info("Running server", zap.String("address", flagRunAddr))
	return http.ListenAndServe(flagRunAddr, logger.RequestLogger(gzipMiddleware(appInstance.webhook)))
}
