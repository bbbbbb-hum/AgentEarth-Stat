package jobs

import (
	"AgentEarth-Stat/cron/internal/redis"
	"AgentEarth-Stat/cron/internal/svc"
	"AgentEarth-Stat/models/users"
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type UserBalanceJob struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUserBalanceJob(ctx context.Context, svcCtx *svc.ServiceContext) *UserBalanceJob {
	return &UserBalanceJob{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (j *UserBalanceJob) Run() {
	//扫描redis中上一个小时所有用户使用量增量key，并获取其值
	keys, err := redis.ScanPrevHourUserUsageIncrementKeys(j.svcCtx)
	if err != nil {
		j.Errorf("Scan redis user usage increment keys failed: %v", err)
		return
	}
	var num int
	for _, key := range keys {
		userId := redis.ExtractUserIdFromKey(key)
		dateStr := redis.ExtractDateFromKey(key)
		day, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			j.Errorf("Parse date failed: %v, dateStr: %s", err, dateStr)
			continue
		}
		usageIncrement, err := redis.GetValueByKey(j.svcCtx, key)
		if err != nil {
			j.Errorf("Get user usage increment failed: %v, key: %s", err, key)
			continue
		}
		j.Infof("User usage increment: userId=%s, date=%s, value=%f", userId, dateStr, usageIncrement)

		// 查询数据库中当天的用户账户数据
		userBalanceModel := j.svcCtx.UserBalanceStatisticDailyModel
		userBalance, err := userBalanceModel.FindOneByDayUserId(j.ctx, day, userId)
		if err != nil && !errors.Is(err, sqlx.ErrNotFound) {
			j.Errorf("Query user balance failed: %v", err)
			continue
		}

		if userBalance == nil {
			// 查询最后一天的用户账户数据，用于获取余额初始值
			lastDayBalance, err := userBalanceModel.FindOneLastDayByUserId(j.ctx, userId)
			if err != nil && !errors.Is(err, sqlx.ErrNotFound) {
				j.Errorf("Query last day user balance failed: %v, userId: %s", err, userId)
				continue
			}
			var initialBalance float64 = 0
			if lastDayBalance != nil {
				initialBalance = lastDayBalance.Balance
			}
			// 创建当天用户余额记录
			userBalance = &users.AeUserBalanceStatisticDaily{
				UserId:  userId,
				Day:     day,
				Balance: initialBalance,
			}
			_, err = userBalanceModel.Insert(j.ctx, userBalance)
			if err != nil {
				j.Errorf("Insert user balance failed: %v, userId: %s, day: %s", err, userId, dateStr)
				continue
			}
			// 重新查询以获取插入后的记录（包含Id）
			userBalance, err = userBalanceModel.FindOneByDayUserId(j.ctx, day, userId)
			if err != nil {
				j.Errorf("Query inserted user balance failed: %v, userId: %s, day: %s", err, userId, dateStr)
				continue
			}
		}

		// 计算扣减后的余额并存入数据库
		userBalanceCalculate := decimal.NewFromFloat(userBalance.Balance)
		usageIncrementDecimal := decimal.NewFromFloat(usageIncrement)
		newBalance, _ := userBalanceCalculate.Sub(usageIncrementDecimal).Float64()

		userBalance.Balance = newBalance
		err = userBalanceModel.Update(j.ctx, userBalance)
		if err != nil {
			j.Errorf("Update user balance failed: %v, userId: %s, day: %s", err, userId, dateStr)
			continue
		}
		j.Infof("Updated user balance: userId=%s, day=%s, oldBalance=%f, usageIncrement=%f, newBalance=%f",
			userId, dateStr, userBalanceCalculate.InexactFloat64(), usageIncrement, newBalance)

		// 处理成功后删除Redis key，避免重复处理
		if err := redis.DeleteKey(j.svcCtx, key); err != nil {
			j.Errorf("Delete redis key failed: %v, key: %s", err, key)
		}
		num++
	}
	j.Infof("Finished processing user balance job,Time:%s, Count:%d", time.Now().Format("2006-01-02 15:04:05"), num)
}
