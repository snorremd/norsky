package firehose

import (
	"context"
	"sync"

	log "github.com/sirupsen/logrus"
)

type ParallelProcessor struct {
	maxWorkers  int
	workerQueue chan *RawMessage
	processors  []*PostProcessor
	postChan    chan interface{} // Channel to send processed posts
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewParallelProcessor(ctx context.Context, maxWorkers int, maxQueueSize int, config FirehoseConfig, postChan chan interface{}) *ParallelProcessor {
	ctx, cancel := context.WithCancel(ctx)

	// Setup new parallel processor with maxWorkers go routines
	pp := &ParallelProcessor{
		maxWorkers:  maxWorkers,
		workerQueue: make(chan *RawMessage, maxQueueSize),
		processors:  make([]*PostProcessor, maxWorkers),
		postChan:    postChan,
		ctx:         ctx,
		cancel:      cancel,
	}

	// Create workers
	for i := 0; i < maxWorkers; i++ {
		pp.processors[i] = NewPostProcessor(ctx, config, postChan)
	}

	return pp
}

func (pp *ParallelProcessor) start() {
	for i, processor := range pp.processors {
		go pp.startWorker(i, processor)
	}
}

func (pp *ParallelProcessor) startWorker(id int, processor *PostProcessor) {
	pp.wg.Add(1)
	defer pp.wg.Done() // Ensure we mark the worker as done when we exit

	for {
		select {
		case <-pp.ctx.Done():
			log.Infof("Worker %d: Shutting down", id)
			return
		case msg := <-pp.workerQueue:
			if err := processor.processPost(msg); err != nil {
				log.Errorf("Worker %d: Error processing message: %v", id, err)
			}
		}
	}
}
