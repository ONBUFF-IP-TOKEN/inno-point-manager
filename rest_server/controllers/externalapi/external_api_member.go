package externalapi

// 포인트 맴버 정보 요청
// func (o *ExternalAPI) GetPointMember(c echo.Context) error {
// 	ctx := base.GetContext(c).(*context.PointManagerContext)

// 	params := context.NewPointMemberInfo()
// 	if err := ctx.EchoContext.Bind(params); err != nil {
// 		log.Error(err)
// 		return base.BaseJSONInternalServerError(c, err)
// 	}

// 	if err := params.CheckValidate(false); err != nil {
// 		return c.JSON(http.StatusOK, err)
// 	}

// 	return commonapi.GetPointMember(params, ctx)
// }
