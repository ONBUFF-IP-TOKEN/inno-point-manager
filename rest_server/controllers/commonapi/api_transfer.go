package commonapi

import (
	"net/http"

	"github.com/ONBUFF-IP-TOKEN/baseapp/base"
	"github.com/ONBUFF-IP-TOKEN/inno-point-manager/rest_server/controllers/commonapi/inner"
	"github.com/ONBUFF-IP-TOKEN/inno-point-manager/rest_server/controllers/context"
	"github.com/ONBUFF-IP-TOKEN/inno-point-manager/rest_server/controllers/resultcode"
	"github.com/ONBUFF-IP-TOKEN/inno-point-manager/rest_server/controllers/token_manager_server"
	"github.com/ONBUFF-IP-TOKEN/inno-point-manager/rest_server/model"
	"github.com/labstack/echo"
)

func PostCoinTransferFromParentWallet(params *context.ReqCoinTransferFromParentWallet, ctx *context.PointManagerContext) error {
	resp := new(base.BaseResponse)
	resp.Success()

	if err := inner.TransferFromParentWallet(params, true); err != nil {
		resp = err
	}

	return ctx.EchoContext.JSON(http.StatusOK, resp)
}

func PostCoinTransferFromUserWallet(params *context.ReqCoinTransferFromUserWallet, ctx *context.PointManagerContext) error {
	resp := new(base.BaseResponse)
	resp.Success()

	if !model.GetExternalTransferEnable() {
		resp.SetReturn(resultcode.Result_Error_IsCoinTransferExternalMaintenance)
		return ctx.EchoContext.JSON(http.StatusOK, resp)
	}

	params.Target = context.From_user_to_other_wallet
	if err := inner.TransferFromUserWallet(params, true); err != nil {
		resp = err
	}

	return ctx.EchoContext.JSON(http.StatusOK, resp)
}

// 코인 외부 지갑 전송 중인 상태 정보 요청
func GetCoinTransferExistInProgress(params *context.GetCoinTransferExistInProgress, ctx *context.PointManagerContext) error {
	resp := new(base.BaseResponse)
	resp.Success()

	if res := inner.IsExistInprogressTransferFromParentWallet(params); res != nil {
		resp = res
	}
	if resp.Return != 0 {
		if res := inner.IsExistInprogressTransferFromUserWallet(params); res != nil {
			resp = res
		}
	}

	return ctx.EchoContext.JSON(http.StatusOK, resp)
}

func PostCoinTransferResultDeposit(params *context.ReqCoinTransferResDeposit, c echo.Context) error {
	resp := new(base.BaseResponse)
	resp.Success()

	if err := inner.TransferResultDeposit(params); err != nil {
		resp = err
	}

	return c.JSON(http.StatusOK, resp)
}

func PostCoinTransferResultWithdrawal(params *context.ReqCoinTransferResWithdrawal, c echo.Context) error {
	resp := new(base.BaseResponse)
	resp.Success()

	if err := inner.TransferResultWithdrawal(params); err != nil {
		resp = err
	}

	return c.JSON(http.StatusOK, resp)
}

func GetCoinFee(params *context.ReqCoinFee, c echo.Context) error {
	resp := new(base.BaseResponse)
	resp.Success()

	req := &token_manager_server.ReqCoinFee{
		Symbol: params.Symbol,
	}

	if res, err := token_manager_server.GetInstance().GetCoinFee(req); err != nil {
		resp.SetReturn(resultcode.ResultInternalServerError)
	} else {
		if res.Return != 0 { // token manager 전송 에러
			resp.Return = res.Return
			resp.Message = res.Message
		} else {
			resp.Value = res.ResCoinFeeInfoValue
		}
	}

	return c.JSON(http.StatusOK, resp)
}

func GetBalance(params *context.ReqBalance, c echo.Context) error {
	resp := new(base.BaseResponse)
	resp.Success()

	req := &token_manager_server.ReqBalance{
		Symbol:  params.Symbol,
		Address: params.Address,
	}

	if res, err := token_manager_server.GetInstance().GetBalance(req); err != nil {
		resp.SetReturn(resultcode.ResultInternalServerError)
	} else {
		if res.Return != 0 { // token manager 전송 에러
			resp.Return = res.Return
			resp.Message = res.Message
		} else {
			resp.Value = res.ResReqBalanceValue
		}
	}

	return c.JSON(http.StatusOK, resp)
}

func PostCoinReload(params *context.CoinReload, ctx *context.PointManagerContext) error {
	resp := inner.CoinReload(params)
	return ctx.EchoContext.JSON(http.StatusOK, resp)
}
