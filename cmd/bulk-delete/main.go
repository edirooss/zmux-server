package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/edirooss/zmux-server/internal/service"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	// CLI flags
	start := flag.Int("start", 0, "start of channel ID range")
	end := flag.Int("end", 0, "end of channel ID range")
	flag.Parse()

	if *start == 0 || *end == 0 || *end < *start {
		fmt.Println("Usage: ./bulk-delete -start=<start_id> -end=<end_id>")
		os.Exit(1)
	}

	log := buildLogger()
	log = log.Named("main")

	svc, err := service.NewChannelService(log)
	if err != nil {
		log.Fatal("channel service creation failed", zap.Error(err))
	}

	total := (*end - *start) + 1
	for idx, id := 0, *start; id <= *end; idx, id = idx+1, id+1 {
		iterStart := time.Now()

		if err := svc.DeleteChannel(context.TODO(), int64(id)); err != nil {
			log.Fatal("channel deletion failed",
				zap.Int("channelID", id),
				zap.Error(err),
			)
		}

		log.Info("channel deleted",
			zap.Int("channelID", id),
			zap.Int("deleted", idx+1),
			zap.Int("total", total),
			zap.Duration("took", time.Since(iterStart)),
		)
	}
}

func buildLogger() *zap.Logger {
	logConfig := zap.NewDevelopmentConfig()
	logConfig.EncoderConfig.TimeKey = ""
	logConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	logConfig.DisableStacktrace = true
	logConfig.DisableCaller = true
	logConfig.Level.SetLevel(zap.DebugLevel)
	return zap.Must(logConfig.Build())
}
