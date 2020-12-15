package main

import (
	"container/list"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	MSS               = 512
	MaxSequenceNumber = 102400
	TimeOut = 1*time.Second
)

const (
	ServerIP   = "127.0.0.1"
	ServerPort = 3456
)

const (
	Establish = iota
	Maintain
	Release
)

type Segment struct {
	Seq          uint32 // Sequence Number
	Ack          uint32 // Acknowledgment Number
	ConnectionID uint16 // Connection Identifier
	Flag         uint16 // Flag Field:13 unused bit and ACK SYN FIN
	Data         []byte // Data Field
}

const (
	ACK = 0x01 << 1
	SYN = 0x01 << 2
	FIN = 0x01 << 3
)

type Queue struct {
	list  *list.List
	mutex sync.Mutex
}

func GetQueue() *Queue {
	return &Queue{
		list: list.New(),
	}
}

func (queue *Queue) Push(data interface{}) {
	if data == nil {
		return
	}
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	queue.list.PushBack(data)
}

func (queue *Queue) Front() interface{} {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	if element := queue.list.Front(); element != nil {
		return element.Value
	}
	return nil
}

func (queue *Queue) Pop() error {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	if element := queue.list.Front(); element != nil {
		queue.list.Remove(element)
		return nil
	}
	return errors.New("pop failed")
}

func (queue *Queue) Clear() {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	for element := queue.list.Front(); element != nil; {
		elementNext := element.Next()
		queue.list.Remove(element)
		element = elementNext
	}
}

func (queue *Queue) Len() int {
	return queue.list.Len()
}

func (queue *Queue) Show() {
	for item := queue.list.Front(); item != nil; item = item.Next() {
		fmt.Println(item.Value)
	}
}
