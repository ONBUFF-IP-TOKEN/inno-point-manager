package model

import (
	"time"

	"github.com/ONBUFF-IP-TOKEN/basenet"
	"github.com/ONBUFF-IP-TOKEN/baseutil/log"
	"github.com/ONBUFF-IP-TOKEN/inno-point-manager/rest_server/controllers/context"
)

var gCommand *Command

const (
	Command_DailyAppPoint = 0
)

type Command struct {
	command chan *basenet.CommandData
}

func GetCmd() *Command {
	if gCommand == nil {
		gCommand = new(Command)
		gCommand.command = make(chan *basenet.CommandData)
		gCommand.StartCmd()
	}

	return gCommand
}

func (o *Command) GetAppPointCmdChannel() chan *basenet.CommandData {
	return o.command
}

func (o *Command) StartCmd() {
	context.GetChanInstance().Put(context.Channel_AppPoint, o.command)

	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)

		defer func() {
			ticker.Stop()
		}()

		for {
			select {
			case ch := <-o.command:
				o.CommandProc(ch)
			case <-ticker.C:
			}
		}
	}()
}

func (o *Command) CommandProc(data *basenet.CommandData) error {
	if data.Data != nil {
		start := time.Now()

		switch data.CommandType {
		case Command_DailyAppPoint:
			o.AddDailyAppPoint(data.Data, data.Callback)
		}

		end := time.Now()
		log.Debug("cmd.kind:", data.CommandType, ",elapsed", end.Sub(start))
	}

	return nil
}

func (o *Command) AddDailyAppPoint(data interface{}, cb chan interface{}) {
	// daily app point
	dailyAppPoint := data.(*context.DailyAppPoint)

	if dailyAppPoint.PointType == context.PointType_EarnPoint {
		lockKey := MakeDailyAppPointLockKey(dailyAppPoint.AppId, dailyAppPoint.PointId)
		if unLock, err := AutoLock(lockKey); err != nil {
			log.Errorf("redis lock fail [lockkey:%v][err:%v]", lockKey, err)
			log.Errorf("daily app point leak [app_id:%v][point_id:%v][adjust_quantity:%v]", dailyAppPoint.AppId, dailyAppPoint.PointId, dailyAppPoint.AdjustQuantity)
			return
		} else {
			defer unLock()
		}

		key := MakeDailyAppPointKey(dailyAppPoint.AppId, dailyAppPoint.PointId)

		cachePoint, err := GetDB().GetCacheDailyAppPoint(key)
		if err != nil {
			log.Infof("GetCacheDailyAppPoint [key:%v][err:%v]", key, err)
			cachePoint = dailyAppPoint
		} else {
			if dailyAppPoint.AdjustQuantity > 0 { // app point 변화량은 양수만 누적한다. 교환량은 절대값으로 누적한다.
				cachePoint.AdjustQuantity += dailyAppPoint.AdjustQuantity
				cachePoint.AdjustExchangeQuantity += dailyAppPoint.AdjustQuantity
			} else {
				cachePoint.AdjustExchangeQuantity += -dailyAppPoint.AdjustQuantity
			}
		}

		if err := GetDB().SetCacheDailyAppPoint(key, cachePoint); err != nil {
			log.Errorf("SetCacheDailyAppPoint [key:%v][err:%v]", key, err)
			log.Errorf("daily app point leak [app_id:%v][point_id:%v][adjust_quantity:%v]", dailyAppPoint.AppId, dailyAppPoint.PointId, dailyAppPoint.AdjustQuantity)
		} else {
			NewDaliyAppPointUnit(dailyAppPoint.AppId, dailyAppPoint.PointId)
		}
	}
}
