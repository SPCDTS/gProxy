package utils

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestPool(t *testing.T) {
	// 1. 没有存活goroutine的话，Accept将无人执行，协程池将无法启动
	// 2. 如果加入协程池的workA又会产生更多的workB，且workB执行速度慢，
	//   可能最终会导致workA占用了全部协程，且阻塞在Go方法中；工作等待队列
	//   应设置地大一些
	p := NewGoPool(10, 1, 20)
	wg := sync.WaitGroup{}
	N := 50
	time.Sleep(1500 * time.Millisecond)
	wg.Add(1)
	work := func() {
		fmt.Println("xx")
		wg.Done()
	}
	p.Go(work)
	wg.Wait()
	for i := 1; i < N; i++ {
		b := i
		p.Go(func() { fmt.Println(b) })
	}
	time.Sleep(500 * time.Millisecond)
}
