package schedule

import (
	"sync"
	"time"

	"github.com/ONBUFF-IP-TOKEN/baseutil/log"
	"github.com/ONBUFF-IP-TOKEN/inno-point-manager/rest_server/config"
	"github.com/ONBUFF-IP-TOKEN/inno-point-manager/rest_server/controllers/context"
	"github.com/ONBUFF-IP-TOKEN/inno-point-manager/rest_server/model"
)

var gSwapExpireScheduler *SwapExpireScheduler
var onceSwapExpireScheduler sync.Once

type SwapExpireScheduler struct {
	Running     bool  //true:스케쥴실행중 , false:스케쥴중지
	DebugMode   bool  //중간중간 로그찍을부분이있을때 true
	ExpireCycle int64 // 만료 시간 second
}

func InitSwapExpireScheduler(conf *config.ServerConfig) *SwapExpireScheduler {
	schedule, ok := conf.ScheduleMap["swap_expire_scheduler"]
	if ok && schedule.Enable {
		onceSwapExpireScheduler.Do(func() {
			gSwapExpireScheduler = new(SwapExpireScheduler)
			gSwapExpireScheduler.Running = true
			gSwapExpireScheduler.ExpireCycle = schedule.ExpireCycle
			gSwapExpireScheduler.Run(schedule.TermSec)
		})
	}
	return gSwapExpireScheduler
}

func (o *SwapExpireScheduler) SetDebugMode(enable bool) {
	o.DebugMode = enable
}

func (o *SwapExpireScheduler) SetRunning(enable bool) {
	o.Running = enable
}

func (o *SwapExpireScheduler) Run(sec int64) {
	ticker := time.NewTicker(time.Duration(sec) * time.Second)
	go func() {
		for t := range ticker.C {
			if o.DebugMode {
				log.Debugf("SwapExpireScheduler : %v", t)
			}
			if o.Running {
				o.ScheduleProcess()
			}
		}
	}()
}

func (o *SwapExpireScheduler) ScheduleProcess() {
	// redis의 "SWAP-WALLET" 정보들 중에 일정 시간이 지나도 '수수료 입금 시작' 진행이 되고있지 않은 내역은 만료 처리 한다.
	//startTime := time.Now().UnixMilli()
	_, list, err := model.GetDB().CacheGetSwapWallets()
	if err != nil {
		log.Errorf("CacheGetSwapWallets err : %v", err)
		return
	}

	for _, value := range list {
		// 수수료 전송 시작 상태가 아닌 정보중에 10분이 지난 정보는 swap 종료하고 삭제 처리한다.
		if value.CreateAt+o.ExpireCycle < time.Now().UTC().Unix() && value.TxStatus < context.SWAP_status_fee_transfer_start {
			log.Debugf("swap expire addr : %v, time:%v", value.WalletAddress, time.Unix(value.CreateAt, 0).Format(time.RFC3339))

			if value.TxType == context.EventID_toCoin {
				// 현재 레디스에 포인트가 쌓이고 있을수 있으니 최종값으로 디비에 저장하고 스왑 포인트 복구 처리 해준다
				pointKey := model.MakeMemberPointListKey(value.MUID)
				mePointInfo, err := model.GetDB().GetCacheMemberPointList(pointKey)
				if err != nil {
					if _, points, err := model.GetDB().USPPO_GetList_MemberPoints(value.MUID, value.DatabaseID); err != nil {
						log.Errorf("GetPointAppList error : %v", err)
					} else {
						if point, ok := points[value.PointID]; ok {
							value.PreviousPointQuantity = point.Quantity
							value.AdjustPointQuantity = -value.AdjustPointQuantity
							value.PointQuantity = value.PreviousPointQuantity + value.AdjustPointQuantity
						}
					}
				} else {
					// redis에 존재 한다면 강제로 db에 먼저 write
					for _, point := range mePointInfo.Points {
						var eventID context.EventID_type
						if point.AdjustQuantity >= 0 {
							eventID = context.EventID_add
						} else {
							eventID = context.EventID_sub
						}

						if point.AdjustQuantity != 0 {
							if todayAcqQuantity, resetDate, err := model.GetDB().UpdateAppPoint(mePointInfo.DatabaseID, mePointInfo.MUID, point.PointID,
								point.PreQuantity, point.AdjustQuantity, point.Quantity, context.LogID_cp, eventID); err != nil {
								log.Errorf("UpdateAppPoint error : %v", err)
							} else {
								//현재 일일 누적량, 날짜 업데이트
								point.TodayQuantity = todayAcqQuantity
								point.ResetDate = resetDate

								point.AdjustQuantity = 0
								point.PreQuantity = point.Quantity
							}
						} else {
							point.AdjustQuantity = 0
							point.PreQuantity = point.Quantity
						}

						// swap point quantity에 업데이트
						if value.PointID == point.PointID && value.MUID == mePointInfo.MUID {
							value.PreviousPointQuantity = point.Quantity
							value.AdjustPointQuantity = -value.AdjustPointQuantity
							value.PointQuantity = value.PreviousPointQuantity + value.AdjustPointQuantity
						}
					}

					model.GetDB().DelCacheMemberPointList(pointKey)
				}

				if err := model.GetDB().USPAU_XchgCmplt_Goods(value, time.Now().Format("2006-01-02 15:04:05.000"), false); err != nil {
					log.Errorf("USPAU_XchgCmplt_Goods err : %v, txid:%v wallet:%v", err, value.TxID, value.WalletAddress)
				} else {
					if err = model.GetDB().CacheDelSwapWallet(value.WalletAddress); err != nil {
						log.Errorf("CacheDelSwapWallet err:%v, wallet:%v", err, value.WalletAddress)
					}
				}
			}

		}
	}
	//log.Debugf("swap expire checktime :%v", time.Now().UnixMilli()-startTime)
}
