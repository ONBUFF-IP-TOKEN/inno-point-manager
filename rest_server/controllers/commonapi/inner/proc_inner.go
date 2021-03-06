package inner

import (
	"errors"
	"strings"
	"time"

	"github.com/ONBUFF-IP-TOKEN/baseutil/log"
	"github.com/ONBUFF-IP-TOKEN/inno-point-manager/rest_server/controllers/context"
	"github.com/ONBUFF-IP-TOKEN/inno-point-manager/rest_server/controllers/resultcode"
	"github.com/ONBUFF-IP-TOKEN/inno-point-manager/rest_server/model"

	uuid "github.com/satori/go.uuid"
)

func UpdateAppPoint(req *context.ReqPointAppUpdate, appId int64) (*context.Point, error) {
	// 1. redis lock
	Lockkey := model.MakeMemberPointListLockKey(req.MUID)
	mutex := model.GetDB().RedSync.NewMutex(Lockkey)
	if err := mutex.Lock(); err != nil {
		log.Error(err)
		return nil, err
	}

	defer func() {
		if ok, err := mutex.Unlock(); !ok || err != nil {
			if err != nil {
				log.Errorf("unlock err : %v", err)
			}
		}
	}()

	respPoint := new(context.Point)
	// 2. redis에 해당 포인트 정보 존재하는지 check
	key := model.MakeMemberPointListKey(req.MUID)
	pointInfo, err := model.GetDB().GetCacheMemberPointList(key)
	if err != nil {
		// 2-1. redis에 존재하지 않는다면 db에서 로드
		if points, err := model.GetDB().GetPointAppList(req.MUID, req.DatabaseID); err != nil {
			return nil, err
		} else {
			// 2-1-1. Account points 로드
			if accountPoint, err := model.GetDB().GetListAccountPoints(0, req.MUID); err != nil {
				return nil, err
			} else {
				// merge
				for _, point := range points {
					if val, ok := accountPoint[point.PointID]; ok {
						point.TodayQuantity = val.TodayAcqQuantity
						if t, err := time.Parse("2006-01-02T15:04:05Z", val.ResetDate); err != nil {
							log.Errorf("time.Parse [err%v]", err)
							point.ResetDate = time.Now().Format("2006-01-02")
							point.TodayQuantity = 0
						} else {
							if !strings.EqualFold(t.Format("2006-01-02"), time.Now().Format("2006-01-02")) {
								// 날짜 바뀌면
								point.TodayQuantity = 0
								point.ResetDate = time.Now().Format("2006-01-02")
							} else {
								point.ResetDate = t.Format("2006-01-02")
							}
						}
					}
				}
			}

			find := false
			findIdx := 0
			for idx, point := range points {
				if point.PointID == req.PointID {
					if point.Quantity == req.PreQuantity { // last 수량 비교
						//일일 최대 적립 포인트량 비교
						enable := checkTodayPoint(point, appId, &req.AdjustQuantity)
						if !enable {
							err = errors.New(resultcode.ResultCodeText[resultcode.Result_Error_Exceeded_TodayPoints_earned])
							break
						}

						// 포인트가 0보다 작게 되지 않도록 한다.
						if points[idx].Quantity+req.AdjustQuantity < 0 {
							req.AdjustQuantity = -points[idx].Quantity
						}

						points[idx].AdjustQuantity += req.AdjustQuantity
						points[idx].Quantity += req.AdjustQuantity
						points[idx].TodayQuantity += req.AdjustQuantity

						find = true
						findIdx = idx
					} else {
						err = errors.New(resultcode.ResultCodeText[resultcode.Result_Error_NotEqual_PreviousQuantity])
					}
					break
				}
			}

			if err != nil {
				log.Errorf("%v ", err)
				return nil, err
			}
			if !find { // point id를 못찼았을경우
				err = errors.New("invalid point ID")
				log.Errorf("%v ", err)
				return nil, err
			}

			respPoint = points[findIdx]

			pointInfo = &context.PointInfo{
				MyUuid:     uuid.NewV4().String(),
				DatabaseID: req.DatabaseID,

				MUID:   req.MUID,
				Points: points,
			}

			// 2-2. redis 에 write
			if err := model.GetDB().SetCacheMemberPointList(key, pointInfo); err != nil {
				return nil, err
			}

			// 2-3. redis update thread 생성
			model.GetDB().PointDocMtx.Lock()
			model.GetDB().PointDoc[key] = model.NewMemberPointInfo(pointInfo, appId, false)
			model.GetDB().PointDocMtx.Unlock()
		}
	} else {
		// redis 에 존재하면 업데이트
		points := pointInfo.Points

		err = nil
		find := false
		findIdx := 0
		for idx, point := range points {
			if point.PointID == req.PointID {
				if point.Quantity == req.PreQuantity { // last 수량 비교
					//일일 최대 적립 포인트량 비교
					enable := checkTodayPoint(point, appId, &req.AdjustQuantity)
					if !enable {
						err = errors.New(resultcode.ResultCodeText[resultcode.Result_Error_Exceeded_TodayPoints_earned])
						break
					}

					// 포인트가 0보다 작게 되지 않도록 한다.
					if points[idx].Quantity+req.AdjustQuantity < 0 {
						req.AdjustQuantity = -points[idx].Quantity
					}

					points[idx].AdjustQuantity += req.AdjustQuantity
					points[idx].Quantity += req.AdjustQuantity
					points[idx].TodayQuantity += req.AdjustQuantity

					find = true
					findIdx = idx
				} else {
					err = errors.New(resultcode.ResultCodeText[resultcode.Result_Error_NotEqual_PreviousQuantity])
				}
				break
			}
		}
		if err != nil {
			log.Errorf("%v muid:%v", err, req.MUID)
			return nil, err
		}
		if !find { // point id를 못찼았을경우
			err = errors.New("invalid point ID")
			log.Errorf("%v ", err)
			return nil, err
		}

		pointInfo.Points = points
		if err := model.GetDB().SetCacheMemberPointList(key, pointInfo); err != nil {
			return nil, err
		}
		respPoint = points[findIdx]
	}

	return respPoint, nil
}

func LoadPointList(MUID, DatabaseID, appId int64) (*context.PointInfo, error) {
	// 1. redis에 해당 포인트 정보 존재하는지 check
	key := model.MakeMemberPointListKey(MUID)
	pointInfo, err := model.GetDB().GetCacheMemberPointList(key)
	if err != nil {
		// 2-1. redis에 존재하지 않는다면 db에서 로드
		if points, err := model.GetDB().GetPointAppList(MUID, DatabaseID); err != nil {
			return nil, err
		} else {
			// 2-1-1. Account points 로드
			if accountPoint, err := model.GetDB().GetListAccountPoints(0, MUID); err != nil {
				return nil, err
			} else {
				// merge
				for _, point := range points {
					if val, ok := accountPoint[point.PointID]; ok {
						point.TodayQuantity = val.TodayAcqQuantity
						if t, err := time.Parse("2006-01-02T15:04:05Z", val.ResetDate); err != nil {
							log.Errorf("time.Parse [err%v]", err)
						} else {
							point.ResetDate = t.Format("2006-01-02")
						}
					}
				}
			}

			pointInfo = &context.PointInfo{
				MyUuid:     uuid.NewV4().String(),
				DatabaseID: DatabaseID,

				MUID:   MUID,
				Points: points,
			}
		}
	} else {
		// redis에 존재 한다면 내가 관리하는 thread check, 내 관리가 아니면 그냥 값만 리턴
	}

	return pointInfo, nil
}

func LoadPoint(MUID, PointID, DatabaseID, appId int64) (*context.PointInfo, error) {
	// 1. redis에 해당 포인트 정보 존재하는지 check
	key := model.MakeMemberPointListKey(MUID)
	pointInfo, err := model.GetDB().GetCacheMemberPointList(key)
	if err != nil {
		// 2-1. redis에 존재하지 않는다면 db에서 로드
		if points, err := model.GetDB().GetPointAppList(MUID, DatabaseID); err != nil {
			return nil, err
		} else {
			pointInfo = &context.PointInfo{
				MyUuid:     uuid.NewV4().String(),
				DatabaseID: DatabaseID,

				MUID: MUID,
			}

			// 2-1-1. Account points 로드
			if accountPoint, err := model.GetDB().GetListAccountPoints(0, MUID); err != nil {
				return nil, err
			} else {
				// merge
				for _, point := range points {
					if val, ok := accountPoint[point.PointID]; ok {
						point.TodayQuantity = val.TodayAcqQuantity
						if t, err := time.Parse("2006-01-02T15:04:05Z", val.ResetDate); err != nil {
							log.Errorf("time parese error :%v", err)
						} else {
							point.ResetDate = t.Format("2006-01-02")
						}
					}
				}
			}

			pointInfo = &context.PointInfo{
				MyUuid:     uuid.NewV4().String(),
				DatabaseID: DatabaseID,

				MUID:   MUID,
				Points: points,
			}

			// 2-4. 요청한 point id만 응답해준다.
			temp := context.Point{}
			bFind := false
			for _, point := range pointInfo.Points {
				if point.PointID == PointID {
					temp = *point
					bFind = true
					break
				}
			}

			pointInfo.Points = []*context.Point{}
			if bFind {
				pointInfo.Points = append(pointInfo.Points, &temp)
			}
		}
	} else {
		// 요청한 point id만 응답해준다.
		temp := context.Point{}
		bFind := false
		for _, point := range pointInfo.Points {
			if point.PointID == PointID {
				temp = *point
				bFind = true
				break
			}
		}

		pointInfo.Points = []*context.Point{}
		if bFind {
			pointInfo.Points = append(pointInfo.Points, &temp)
		}
	}

	return pointInfo, nil
}

func checkTodayPoint(point *context.Point, appId int64, reqAdjustQuantity *int64) bool {
	return point.TodayQuantity+point.AdjustQuantity+*reqAdjustQuantity < model.GetDB().AppPointsMap[appId].PointsMap[point.PointID].DaliyLimitedAcqQuantity
}
