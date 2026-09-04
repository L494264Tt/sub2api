package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"golang.org/x/sync/singleflight"
)

// 閿欒瀹氫箟
// 娉細ErrInsufficientBalance鍦╮edeem_service.go涓畾涔?// 娉細ErrDailyLimitExceeded/ErrWeeklyLimitExceeded/ErrMonthlyLimitExceeded鍦╯ubscription_service.go涓畾涔?// errBillingCacheUnavailable 鍐呴儴鍝ㄥ叺锛氱敤浜?quota 鏍￠獙璺緞鍦?cache==nil 鏃?// 涓?Redis 鏁呴殰"璧板悓涓€鏉?fail-open + DB 涓€娆℃€ф鏌ョ殑鍒嗘敮銆?
var errBillingCacheUnavailable = fmt.Errorf("billing cache unavailable")

var (
	ErrSubscriptionInvalid       = infraerrors.Forbidden("SUBSCRIPTION_INVALID", "subscription is invalid or expired")
	ErrBillingServiceUnavailable = infraerrors.ServiceUnavailable("BILLING_SERVICE_ERROR", "Billing service temporarily unavailable. Please retry later.")
	// RPM 瓒呴檺閿欒銆俫ateway_handler 璐熻矗鏄犲皠涓?HTTP 429銆?
	ErrGroupRPMExceeded = infraerrors.TooManyRequests("GROUP_RPM_EXCEEDED", "group requests-per-minute limit exceeded")
	ErrUserRPMExceeded  = infraerrors.TooManyRequests("USER_RPM_EXCEEDED", "user requests-per-minute limit exceeded")

	// user 脳 platform quota锛圚TTP 429 Too Many Requests + Retry-After header锛夈€?	// 閫夌敤 429 鑰岄潪 403锛氶檺棰濊€楀敖灞炰簬"鏆傛椂鎬ц祫婧愮敤灏斤紝閲嶈瘯鍙仮澶?鐨勫満鏅紙RFC 6585锛夛紝
	// 澶ч噺 SDK锛堝 OpenAI 鍏煎瀹㈡埛绔級鍙 429 瑙﹀彂鑷姩閫€閬垮苟璇诲彇 Retry-After锛?
	// 鐢?403 浼氳瑙嗕负"鏉冮檺涓嶈冻锛岄噸璇曟棤鎰忎箟"瀵艰嚧瀹㈡埛绔洿鎺ユ姤閿欎笖涓嶉€€閬裤€?
	ErrUserPlatformDailyQuotaExhausted   = infraerrors.TooManyRequests("USER_PLATFORM_DAILY_QUOTA_EXHAUSTED", "Daily usage quota exhausted for this platform.")
	ErrUserPlatformWeeklyQuotaExhausted  = infraerrors.TooManyRequests("USER_PLATFORM_WEEKLY_QUOTA_EXHAUSTED", "Weekly usage quota exhausted for this platform.")
	ErrUserPlatformMonthlyQuotaExhausted = infraerrors.TooManyRequests("USER_PLATFORM_MONTHLY_QUOTA_EXHAUSTED", "Monthly usage quota exhausted for this platform.")
	ErrUserToken1dQuotaExhausted         = infraerrors.TooManyRequests("USER_TOKEN_1D_QUOTA_EXHAUSTED", "Daily GPT token quota exhausted for this user.")
	ErrUserToken7dQuotaExhausted         = infraerrors.TooManyRequests("USER_TOKEN_7D_QUOTA_EXHAUSTED", "7-day rolling GPT token quota exhausted for this user.")
	ErrUserToken30dQuotaExhausted        = infraerrors.TooManyRequests("USER_TOKEN_30D_QUOTA_EXHAUSTED", "30-day rolling GPT token quota exhausted for this user.")
)

// subscriptionCacheData 璁㈤槄缂撳瓨鏁版嵁缁撴瀯锛堝唴閮ㄤ娇鐢級
type subscriptionCacheData struct {
	Status       string
	ExpiresAt    time.Time
	DailyUsage   float64
	WeeklyUsage  float64
	MonthlyUsage float64
	Version      int64
}

// 缂撳瓨鍐欏叆浠诲姟绫诲瀷
type cacheWriteKind int

const (
	cacheWriteSetBalance cacheWriteKind = iota
	cacheWriteSetSubscription
	cacheWriteUpdateSubscriptionUsage
	cacheWriteDeductBalance
	cacheWriteUpdateRateLimitUsage
)

// 寮傛缂撳瓨鍐欏叆宸ヤ綔姹犻厤缃?//
// 鎬ц兘浼樺寲璇存槑锛?// 鍘熷疄鐜板湪璇锋眰鐑矾寰勪腑浣跨敤 goroutine 寮傛鏇存柊缂撳瓨锛屽瓨鍦ㄤ互涓嬮棶棰橈細
// 1. 姣忔璇锋眰鍒涘缓鏂?goroutine锛岄珮骞跺彂涓嬩骇鐢熷ぇ閲忕煭鐢熷懡鍛ㄦ湡 goroutine
// 2. 鏃犳硶鎺у埗骞跺彂鏁伴噺锛屽彲鑳藉鑷?Redis 杩炴帴鑰楀敖
// 3. goroutine 鍒涘缓/閿€姣佸甫鏉ラ澶栧紑閿€
//
// 鏂板疄鐜颁娇鐢ㄥ浐瀹氬ぇ灏忕殑宸ヤ綔姹狅細
// 1. 棰勫垱寤?10 涓?worker goroutine锛岄伩鍏嶉绻佸垱寤洪攢姣?// 2. 浣跨敤甯︾紦鍐茬殑 channel锛?000锛変綔涓轰换鍔￠槦鍒楋紝骞虫粦鍐欏叆宄板€?// 3. 闈為樆濉炲啓鍏ワ紝闃熷垪婊℃椂鍏抽敭浠诲姟鍚屾鍥為€€锛岄潪鍏抽敭浠诲姟涓㈠純骞跺憡璀?// 4. 缁熶竴瓒呮椂鎺у埗锛岄伩鍏嶆參鎿嶄綔闃诲宸ヤ綔姹?
const (
	cacheWriteWorkerCount     = 10              // 宸ヤ綔鍗忕▼鏁伴噺
	cacheWriteBufferSize      = 1000            // 浠诲姟闃熷垪缂撳啿澶у皬
	cacheWriteTimeout         = 2 * time.Second // 鍗曚釜鍐欏叆鎿嶄綔瓒呮椂
	cacheWriteDropLogInterval = 5 * time.Second // 涓㈠純鏃ュ織鑺傛祦闂撮殧
	balanceLoadTimeout        = 3 * time.Second
	userTokenUsageCacheTTL    = 30 * time.Second
	userTokenUsageLoadTimeout = 3 * time.Second
)

// cacheWriteTask 缂撳瓨鍐欏叆浠诲姟
type cacheWriteTask struct {
	kind             cacheWriteKind
	userID           int64
	groupID          int64
	apiKeyID         int64
	balance          float64
	amount           float64
	subscriptionData *subscriptionCacheData
}

// apiKeyRateLimitLoader defines the interface for loading rate limit data from DB.
type apiKeyRateLimitLoader interface {
	GetRateLimitData(ctx context.Context, keyID int64) (*APIKeyRateLimitData, error)
}

type subscriptionCacheInvalidationPubSub interface {
	PublishSubscriptionCacheInvalidation(ctx context.Context, cacheKey string) error
	SubscribeSubscriptionCacheInvalidation(ctx context.Context, handler func(cacheKey string)) error
}

// BillingCacheService 璁¤垂缂撳瓨鏈嶅姟
// 璐熻矗浣欓鍜岃闃呮暟鎹殑缂撳瓨绠＄悊锛屾彁渚涢珮鎬ц兘鐨勮璐硅祫鏍兼鏌?
type BillingCacheService struct {
	cache                 BillingCache
	userRepo              UserRepository
	subRepo               UserSubscriptionRepository
	apiKeyRateLimitLoader apiKeyRateLimitLoader
	userRPMCache          UserRPMCache
	userGroupRateRepo     UserGroupRateRepository
	cfg                   *config.Config
	circuitBreaker        *billingCircuitBreaker
	userPlatformQuotaRepo UserPlatformQuotaRepository
	usageLogRepo          UsageLogRepository

	cacheWriteChan     chan cacheWriteTask
	cacheWriteWg       sync.WaitGroup
	cacheWriteStopOnce sync.Once
	cacheWriteMu       sync.RWMutex
	stopped            atomic.Bool
	balanceLoadSF      singleflight.Group
	quotaLoadSF        singleflight.Group
	tokenQuotaLoadSF   singleflight.Group
	// 涓㈠純鏃ュ織鑺傛祦璁℃暟鍣紙鍑忓皯楂樿礋杞戒笅鏃ュ織鍣煶锛?
	cacheWriteDropFullCount     uint64
	cacheWriteDropFullLastLog   int64
	cacheWriteDropClosedCount   uint64
	cacheWriteDropClosedLastLog int64
}

func (s *BillingCacheService) SetUsageLogRepository(repo UsageLogRepository) {
	if s != nil {
		s.usageLogRepo = repo
	}
}

// NewBillingCacheService 鍒涘缓璁¤垂缂撳瓨鏈嶅姟
func NewBillingCacheService(
	cache BillingCache,
	userRepo UserRepository,
	subRepo UserSubscriptionRepository,
	apiKeyRepo APIKeyRepository,
	userRPMCache UserRPMCache,
	userGroupRateRepo UserGroupRateRepository,
	cfg *config.Config,
	userPlatformQuotaRepo UserPlatformQuotaRepository,
) *BillingCacheService {
	svc := &BillingCacheService{
		cache:                 cache,
		userRepo:              userRepo,
		subRepo:               subRepo,
		apiKeyRateLimitLoader: apiKeyRepo,
		userRPMCache:          userRPMCache,
		userGroupRateRepo:     userGroupRateRepo,
		cfg:                   cfg,
		userPlatformQuotaRepo: userPlatformQuotaRepo,
	}
	svc.circuitBreaker = newBillingCircuitBreaker(cfg.Billing.CircuitBreaker)
	svc.startCacheWriteWorkers()
	return svc
}

// Stop 鍏抽棴缂撳瓨鍐欏叆宸ヤ綔姹?
func (s *BillingCacheService) Stop() {
	s.cacheWriteStopOnce.Do(func() {
		s.stopped.Store(true)

		s.cacheWriteMu.Lock()
		ch := s.cacheWriteChan
		if ch != nil {
			close(ch)
		}
		s.cacheWriteMu.Unlock()

		if ch == nil {
			return
		}
		s.cacheWriteWg.Wait()

		s.cacheWriteMu.Lock()
		if s.cacheWriteChan == ch {
			s.cacheWriteChan = nil
		}
		s.cacheWriteMu.Unlock()
	})
}

func (s *BillingCacheService) startCacheWriteWorkers() {
	ch := make(chan cacheWriteTask, cacheWriteBufferSize)
	s.cacheWriteChan = ch
	for i := 0; i < cacheWriteWorkerCount; i++ {
		s.cacheWriteWg.Add(1)
		go s.cacheWriteWorker(ch)
	}
}

// enqueueCacheWrite 灏濊瘯灏嗕换鍔″叆闃燂紝闃熷垪婊℃椂杩斿洖 false锛堝苟璁板綍鍛婅锛夈€?
func (s *BillingCacheService) enqueueCacheWrite(task cacheWriteTask) (enqueued bool) {
	if s.stopped.Load() {
		s.logCacheWriteDrop(task, "closed")
		return false
	}

	s.cacheWriteMu.RLock()
	defer s.cacheWriteMu.RUnlock()

	if s.cacheWriteChan == nil {
		s.logCacheWriteDrop(task, "closed")
		return false
	}

	select {
	case s.cacheWriteChan <- task:
		return true
	default:
		// 闃熷垪婊℃椂涓嶉樆濉炰富娴佺▼锛屼氦鐢辫皟鐢ㄦ柟鍐冲畾鏄惁鍚屾鍥為€€銆?
		s.logCacheWriteDrop(task, "full")
		return false
	}
}

func (s *BillingCacheService) cacheWriteWorker(ch <-chan cacheWriteTask) {
	defer s.cacheWriteWg.Done()
	for task := range ch {
		ctx, cancel := context.WithTimeout(context.Background(), cacheWriteTimeout)
		switch task.kind {
		case cacheWriteSetBalance:
			s.setBalanceCache(ctx, task.userID, task.balance)
		case cacheWriteSetSubscription:
			s.setSubscriptionCache(ctx, task.userID, task.groupID, task.subscriptionData)
		case cacheWriteUpdateSubscriptionUsage:
			if s.cache != nil {
				if err := s.cache.UpdateSubscriptionUsage(ctx, task.userID, task.groupID, task.amount); err != nil {
					logger.LegacyPrintf("service.billing_cache", "Warning: update subscription cache failed for user %d group %d: %v", task.userID, task.groupID, err)
				}
			}
		case cacheWriteDeductBalance:
			if s.cache != nil {
				if err := s.cache.DeductUserBalance(ctx, task.userID, task.amount); err != nil {
					logger.LegacyPrintf("service.billing_cache", "Warning: deduct balance cache failed for user %d: %v", task.userID, err)
				}
			}
		case cacheWriteUpdateRateLimitUsage:
			if s.cache != nil {
				if err := s.cache.UpdateAPIKeyRateLimitUsage(ctx, task.apiKeyID, task.amount); err != nil {
					logger.LegacyPrintf("service.billing_cache", "Warning: update rate limit usage cache failed for api key %d: %v", task.apiKeyID, err)
				}
			}
		}
		cancel()
	}
}

// cacheWriteKindName 鐢ㄤ簬鏃ュ織涓殑浠诲姟绫诲瀷鏍囪瘑锛屼究浜庢帓鏌ヤ涪寮冨師鍥犮€?
func cacheWriteKindName(kind cacheWriteKind) string {
	switch kind {
	case cacheWriteSetBalance:
		return "set_balance"
	case cacheWriteSetSubscription:
		return "set_subscription"
	case cacheWriteUpdateSubscriptionUsage:
		return "update_subscription_usage"
	case cacheWriteDeductBalance:
		return "deduct_balance"
	case cacheWriteUpdateRateLimitUsage:
		return "update_rate_limit_usage"
	default:
		return "unknown"
	}
}

// logCacheWriteDrop 浣跨敤鑺傛祦鏂瑰紡璁板綍涓㈠純鎯呭喌锛屽苟姹囨€讳涪寮冩暟閲忋€?
func (s *BillingCacheService) logCacheWriteDrop(task cacheWriteTask, reason string) {
	var (
		countPtr *uint64
		lastPtr  *int64
	)
	switch reason {
	case "full":
		countPtr = &s.cacheWriteDropFullCount
		lastPtr = &s.cacheWriteDropFullLastLog
	case "closed":
		countPtr = &s.cacheWriteDropClosedCount
		lastPtr = &s.cacheWriteDropClosedLastLog
	default:
		return
	}

	atomic.AddUint64(countPtr, 1)
	now := time.Now().UnixNano()
	last := atomic.LoadInt64(lastPtr)
	if now-last < int64(cacheWriteDropLogInterval) {
		return
	}
	if !atomic.CompareAndSwapInt64(lastPtr, last, now) {
		return
	}
	dropped := atomic.SwapUint64(countPtr, 0)
	if dropped == 0 {
		return
	}
	logger.LegacyPrintf("service.billing_cache", "Warning: cache write queue %s, dropped %d tasks in last %s (latest kind=%s user %d group %d)",
		reason,
		dropped,
		cacheWriteDropLogInterval,
		cacheWriteKindName(task.kind),
		task.userID,
		task.groupID,
	)
}

// ============================================
// 浣欓缂撳瓨鏂规硶
// ============================================

// GetUserBalance 鑾峰彇鐢ㄦ埛浣欓锛堜紭鍏堜粠缂撳瓨璇诲彇锛?
func (s *BillingCacheService) GetUserBalance(ctx context.Context, userID int64) (float64, error) {
	if s.cache == nil {
		// Redis涓嶅彲鐢紝鐩存帴鏌ヨ鏁版嵁搴?
		return s.getUserBalanceFromDB(ctx, userID)
	}

	// 灏濊瘯浠庣紦瀛樿鍙?
	balance, err := s.cache.GetUserBalance(ctx, userID)
	if err == nil {
		return balance, nil
	}

	// 缂撳瓨鏈懡涓細singleflight 鍚堝苟鍚屼竴 userID 鐨勫苟鍙戝洖婧愯姹傘€?
	value, err, _ := s.balanceLoadSF.Do(strconv.FormatInt(userID, 10), func() (any, error) {
		loadCtx, cancel := context.WithTimeout(context.Background(), balanceLoadTimeout)
		defer cancel()

		balance, err := s.getUserBalanceFromDB(loadCtx, userID)
		if err != nil {
			return nil, err
		}

		// 寮傛寤虹珛缂撳瓨
		_ = s.enqueueCacheWrite(cacheWriteTask{
			kind:    cacheWriteSetBalance,
			userID:  userID,
			balance: balance,
		})
		return balance, nil
	})
	if err != nil {
		return 0, err
	}
	balance, ok := value.(float64)
	if !ok {
		return 0, fmt.Errorf("unexpected balance type: %T", value)
	}
	return balance, nil
}

// getUserBalanceFromDB 浠庢暟鎹簱鑾峰彇鐢ㄦ埛浣欓
func (s *BillingCacheService) getUserBalanceFromDB(ctx context.Context, userID int64) (float64, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("get user balance: %w", err)
	}
	return user.Balance, nil
}

// setBalanceCache 璁剧疆浣欓缂撳瓨
func (s *BillingCacheService) setBalanceCache(ctx context.Context, userID int64, balance float64) {
	if s.cache == nil {
		return
	}
	if err := s.cache.SetUserBalance(ctx, userID, balance); err != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: set balance cache failed for user %d: %v", userID, err)
	}
}

// DeductBalanceCache 鎵ｅ噺浣欓缂撳瓨锛堝悓姝ヨ皟鐢級
func (s *BillingCacheService) DeductBalanceCache(ctx context.Context, userID int64, amount float64) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.DeductUserBalance(ctx, userID, amount)
}

// QueueDeductBalance 寮傛鎵ｅ噺浣欓缂撳瓨
func (s *BillingCacheService) QueueDeductBalance(userID int64, amount float64) {
	if s.cache == nil {
		return
	}
	// 闃熷垪婊℃椂鍚屾鍥為€€锛岄伩鍏嶅叧閿墸鍑忚闈欓粯涓㈠純銆?
	if s.enqueueCacheWrite(cacheWriteTask{
		kind:   cacheWriteDeductBalance,
		userID: userID,
		amount: amount,
	}) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cacheWriteTimeout)
	defer cancel()
	if err := s.DeductBalanceCache(ctx, userID, amount); err != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: deduct balance cache fallback failed for user %d: %v", userID, err)
	}
}

// InvalidateUserBalance 澶辨晥鐢ㄦ埛浣欓缂撳瓨
func (s *BillingCacheService) InvalidateUserBalance(ctx context.Context, userID int64) error {
	if s.cache == nil {
		return nil
	}
	if err := s.cache.InvalidateUserBalance(ctx, userID); err != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: invalidate balance cache failed for user %d: %v", userID, err)
		return err
	}
	return nil
}

// ============================================
// 璁㈤槄缂撳瓨鏂规硶
// ============================================

// GetSubscriptionStatus 鑾峰彇璁㈤槄鐘舵€侊紙浼樺厛浠庣紦瀛樿鍙栵級
func (s *BillingCacheService) GetSubscriptionStatus(ctx context.Context, userID, groupID int64) (*subscriptionCacheData, error) {
	if s.cache == nil {
		return s.getSubscriptionFromDB(ctx, userID, groupID)
	}

	// 灏濊瘯浠庣紦瀛樿鍙?
	cacheData, err := s.cache.GetSubscriptionCache(ctx, userID, groupID)
	if err == nil && cacheData != nil {
		return s.convertFromPortsData(cacheData), nil
	}

	// 缂撳瓨鏈懡涓紝浠庢暟鎹簱璇诲彇
	data, err := s.getSubscriptionFromDB(ctx, userID, groupID)
	if err != nil {
		return nil, err
	}

	// 寮傛寤虹珛缂撳瓨
	_ = s.enqueueCacheWrite(cacheWriteTask{
		kind:             cacheWriteSetSubscription,
		userID:           userID,
		groupID:          groupID,
		subscriptionData: data,
	})

	return data, nil
}

func (s *BillingCacheService) convertFromPortsData(data *SubscriptionCacheData) *subscriptionCacheData {
	return &subscriptionCacheData{
		Status:       data.Status,
		ExpiresAt:    data.ExpiresAt,
		DailyUsage:   data.DailyUsage,
		WeeklyUsage:  data.WeeklyUsage,
		MonthlyUsage: data.MonthlyUsage,
		Version:      data.Version,
	}
}

func (s *BillingCacheService) convertToPortsData(data *subscriptionCacheData) *SubscriptionCacheData {
	return &SubscriptionCacheData{
		Status:       data.Status,
		ExpiresAt:    data.ExpiresAt,
		DailyUsage:   data.DailyUsage,
		WeeklyUsage:  data.WeeklyUsage,
		MonthlyUsage: data.MonthlyUsage,
		Version:      data.Version,
	}
}

// getSubscriptionFromDB 浠庢暟鎹簱鑾峰彇璁㈤槄鏁版嵁
func (s *BillingCacheService) getSubscriptionFromDB(ctx context.Context, userID, groupID int64) (*subscriptionCacheData, error) {
	sub, err := s.subRepo.GetActiveByUserIDAndGroupID(ctx, userID, groupID)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}

	return &subscriptionCacheData{
		Status:       sub.Status,
		ExpiresAt:    sub.ExpiresAt,
		DailyUsage:   sub.DailyUsageUSD,
		WeeklyUsage:  sub.WeeklyUsageUSD,
		MonthlyUsage: sub.MonthlyUsageUSD,
		Version:      sub.UpdatedAt.Unix(),
	}, nil
}

// setSubscriptionCache 璁剧疆璁㈤槄缂撳瓨
func (s *BillingCacheService) setSubscriptionCache(ctx context.Context, userID, groupID int64, data *subscriptionCacheData) {
	if s.cache == nil || data == nil {
		return
	}
	if err := s.cache.SetSubscriptionCache(ctx, userID, groupID, s.convertToPortsData(data)); err != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: set subscription cache failed for user %d group %d: %v", userID, groupID, err)
	}
}

// UpdateSubscriptionUsage 鏇存柊璁㈤槄鐢ㄩ噺缂撳瓨锛堝悓姝ヨ皟鐢級
func (s *BillingCacheService) UpdateSubscriptionUsage(ctx context.Context, userID, groupID int64, costUSD float64) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.UpdateSubscriptionUsage(ctx, userID, groupID, costUSD)
}

// QueueUpdateSubscriptionUsage 寮傛鏇存柊璁㈤槄鐢ㄩ噺缂撳瓨
func (s *BillingCacheService) QueueUpdateSubscriptionUsage(userID, groupID int64, costUSD float64) {
	if s.cache == nil {
		return
	}
	// 闃熷垪婊℃椂鍚屾鍥為€€锛岀‘淇濊闃呯敤閲忓強鏃舵洿鏂般€?
	if s.enqueueCacheWrite(cacheWriteTask{
		kind:    cacheWriteUpdateSubscriptionUsage,
		userID:  userID,
		groupID: groupID,
		amount:  costUSD,
	}) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cacheWriteTimeout)
	defer cancel()
	if err := s.UpdateSubscriptionUsage(ctx, userID, groupID, costUSD); err != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: update subscription cache fallback failed for user %d group %d: %v", userID, groupID, err)
	}
}

// InvalidateSubscription 澶辨晥鎸囧畾璁㈤槄缂撳瓨
func (s *BillingCacheService) InvalidateSubscription(ctx context.Context, userID, groupID int64) error {
	if s.cache == nil {
		return nil
	}
	if err := s.cache.InvalidateSubscriptionCache(ctx, userID, groupID); err != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: invalidate subscription cache failed for user %d group %d: %v", userID, groupID, err)
		return err
	}
	return nil
}

func (s *BillingCacheService) PublishSubscriptionCacheInvalidation(ctx context.Context, cacheKey string) error {
	if s.cache == nil {
		return nil
	}
	pubsub, ok := s.cache.(subscriptionCacheInvalidationPubSub)
	if !ok {
		return nil
	}
	return pubsub.PublishSubscriptionCacheInvalidation(ctx, cacheKey)
}

func (s *BillingCacheService) SubscribeSubscriptionCacheInvalidation(ctx context.Context, handler func(cacheKey string)) error {
	if s.cache == nil {
		return nil
	}
	pubsub, ok := s.cache.(subscriptionCacheInvalidationPubSub)
	if !ok {
		return nil
	}
	return pubsub.SubscribeSubscriptionCacheInvalidation(ctx, handler)
}

// InvalidateAPIKeyRateLimit invalidates the Redis rate-limit usage cache for an API key.
func (s *BillingCacheService) InvalidateAPIKeyRateLimit(ctx context.Context, keyID int64) error {
	if s.cache == nil {
		return nil
	}
	if err := s.cache.InvalidateAPIKeyRateLimit(ctx, keyID); err != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: invalidate api key rate limit cache failed for key %d: %v", keyID, err)
		return err
	}
	return nil
}

// ============================================
// API Key 闄愰€熺紦瀛樻柟娉?// ============================================

// checkAPIKeyRateLimits checks rate limit windows for an API key.
// It loads usage from Redis cache (falling back to DB on cache miss),
// resets expired windows in-memory and triggers async DB reset,
// and returns an error if any window limit is exceeded.
func (s *BillingCacheService) checkAPIKeyRateLimits(ctx context.Context, apiKey *APIKey) error {
	if s.cache == nil {
		// No cache: fall back to reading from DB directly
		if s.apiKeyRateLimitLoader == nil {
			return nil
		}
		data, err := s.apiKeyRateLimitLoader.GetRateLimitData(ctx, apiKey.ID)
		if err != nil {
			return nil // Don't block requests on DB errors
		}
		return s.evaluateRateLimits(ctx, apiKey, data.Usage5h, data.Usage1d, data.Usage7d,
			data.Window5hStart, data.Window1dStart, data.Window7dStart)
	}

	cacheData, err := s.cache.GetAPIKeyRateLimit(ctx, apiKey.ID)
	if err != nil {
		// Cache miss: load from DB and populate cache
		if s.apiKeyRateLimitLoader == nil {
			return nil
		}
		dbData, dbErr := s.apiKeyRateLimitLoader.GetRateLimitData(ctx, apiKey.ID)
		if dbErr != nil {
			return nil // Don't block requests on DB errors
		}
		// Build cache entry from DB data
		cacheEntry := &APIKeyRateLimitCacheData{
			Usage5h: dbData.Usage5h,
			Usage1d: dbData.Usage1d,
			Usage7d: dbData.Usage7d,
		}
		if dbData.Window5hStart != nil {
			cacheEntry.Window5h = dbData.Window5hStart.Unix()
		}
		if dbData.Window1dStart != nil {
			cacheEntry.Window1d = dbData.Window1dStart.Unix()
		}
		if dbData.Window7dStart != nil {
			cacheEntry.Window7d = dbData.Window7dStart.Unix()
		}
		_ = s.cache.SetAPIKeyRateLimit(ctx, apiKey.ID, cacheEntry)
		cacheData = cacheEntry
	}

	var w5h, w1d, w7d *time.Time
	if cacheData.Window5h > 0 {
		t := time.Unix(cacheData.Window5h, 0)
		w5h = &t
	}
	if cacheData.Window1d > 0 {
		t := time.Unix(cacheData.Window1d, 0)
		w1d = &t
	}
	if cacheData.Window7d > 0 {
		t := time.Unix(cacheData.Window7d, 0)
		w7d = &t
	}
	return s.evaluateRateLimits(ctx, apiKey, cacheData.Usage5h, cacheData.Usage1d, cacheData.Usage7d, w5h, w1d, w7d)
}

// evaluateRateLimits checks usage against limits, triggering async resets for expired windows.
func (s *BillingCacheService) evaluateRateLimits(ctx context.Context, apiKey *APIKey, usage5h, usage1d, usage7d float64, w5h, w1d, w7d *time.Time) error {
	needsReset := false

	// Reset expired windows in-memory for check purposes
	if IsWindowExpired(w5h, RateLimitWindow5h) {
		usage5h = 0
		needsReset = true
	}
	if IsWindowExpired(w1d, RateLimitWindow1d) {
		usage1d = 0
		needsReset = true
	}
	if IsWindowExpired(w7d, RateLimitWindow7d) {
		usage7d = 0
		needsReset = true
	}

	// Trigger async DB reset if any window expired
	if needsReset {
		keyID := apiKey.ID
		go func() {
			resetCtx, cancel := context.WithTimeout(context.Background(), cacheWriteTimeout)
			defer cancel()
			if s.apiKeyRateLimitLoader != nil {
				// Use the repo directly - reset then reload cache
				if loader, ok := s.apiKeyRateLimitLoader.(interface {
					ResetRateLimitWindows(ctx context.Context, id int64) error
				}); ok {
					if err := loader.ResetRateLimitWindows(resetCtx, keyID); err != nil {
						logger.LegacyPrintf("service.billing_cache", "Warning: reset rate limit windows failed for api key %d: %v", keyID, err)
					}
				}
			}
			// Invalidate cache so next request loads fresh data
			if s.cache != nil {
				if err := s.cache.InvalidateAPIKeyRateLimit(resetCtx, keyID); err != nil {
					logger.LegacyPrintf("service.billing_cache", "Warning: invalidate rate limit cache failed for api key %d: %v", keyID, err)
				}
			}
		}()
	}

	// Check limits
	if apiKey.RateLimit5h > 0 && usage5h >= apiKey.RateLimit5h {
		return ErrAPIKeyRateLimit5hExceeded
	}
	if apiKey.RateLimit1d > 0 && usage1d >= apiKey.RateLimit1d {
		return ErrAPIKeyRateLimit1dExceeded
	}
	if apiKey.RateLimit7d > 0 && usage7d >= apiKey.RateLimit7d {
		return ErrAPIKeyRateLimit7dExceeded
	}
	return nil
}

// QueueUpdateAPIKeyRateLimitUsage asynchronously updates rate limit usage in the cache.
func (s *BillingCacheService) QueueUpdateAPIKeyRateLimitUsage(apiKeyID int64, cost float64) {
	if s.cache == nil {
		return
	}
	s.enqueueCacheWrite(cacheWriteTask{
		kind:     cacheWriteUpdateRateLimitUsage,
		apiKeyID: apiKeyID,
		amount:   cost,
	})
}

// IncrementUserPlatformQuotaUsage 鍚屾绱姞 user 脳 platform usage 鍒?Redis 缂撳瓨銆?//
// 璁捐锛氬悓姝ュ啓鍏ヨ€岄潪寮傛鍏ラ槦銆傚悓姝ュ啓纭繚涓嬫 preflight 绔嬪嵆鐪嬪埌鏈€鏂?usage锛?// 鎶?TOCTOU 瓒呮敮绐楀彛闄愬埗鍦ㄥ苟鍙?in-flight 璇锋眰鏁伴噺鍐咃紙鑰岄潪闅忔椂闂存棤闄愮疮绉級銆?// 鍐欏欢杩熼€氬父 < 1ms锛堟湰鍦?Redis锛夛紝鎹㈠彇 quota 瑙嗗浘瀹炴椂鎬х殑鍙栬垗鍚堢悊銆?//
// Redis 鍐欏け璐ョ敤 ALERT 绾?log锛汥B 鎸佷箙鍖栫敱 caller 鍗曠嫭 goroutine 鍏滃簳锛坓ateway_service.go锛夈€?
func (s *BillingCacheService) IncrementUserPlatformQuotaUsage(userID int64, platform string, cost float64) {
	if s.cache == nil {
		return
	}
	if platform == "" || cost <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cacheWriteTimeout)
	defer cancel()
	ttl := time.Duration(s.cfg.Billing.UserPlatformQuotaCacheTTLSeconds) * time.Second
	markDirty := s.cfg.Database.UserPlatformQuotaFlusherEnabled
	if err := s.cache.IncrUserPlatformQuotaUsageCache(ctx, userID, platform, cost, ttl, markDirty); err != nil {
		logger.LegacyPrintf("service.billing_cache",
			"ALERT: incr user platform quota cache failed user=%d platform=%s cost=%f: %v",
			userID, platform, cost, err)
	}
}

// ============================================
// 缁熶竴妫€鏌ユ柟娉?// ============================================

// CheckBillingEligibility 妫€鏌ョ敤鎴锋槸鍚︽湁璧勬牸鍙戣捣璇锋眰
// 浣欓妯″紡锛氭鏌ョ紦瀛樹綑棰?> 0
// 璁㈤槄妯″紡锛氭鏌ョ紦瀛樼敤閲忔湭瓒呰繃闄愰锛圙roup闄愰浠庡弬鏁颁紶鍏ワ級
// platform 涓鸿姹傜殑鐩爣骞冲彴锛堝 "anthropic"锛夛紝浼犵┖涓?"" 鏃惰烦杩?user 脳 platform quota 妫€鏌ャ€?
func (s *BillingCacheService) CheckBillingEligibility(ctx context.Context, user *User, apiKey *APIKey, group *Group, subscription *UserSubscription, platform string) error {
	requestedModel, _ := RequestedModelFromContext(ctx)
	if err := s.checkUserTokenQuotaEligibility(ctx, user, requestedModel); err != nil {
		return err
	}
	// 绠€鏄撴ā寮忥細璺宠繃鎵€鏈夎璐规鏌?
	if s.cfg.RunMode == config.RunModeSimple {
		return nil
	}
	if s.circuitBreaker != nil && !s.circuitBreaker.Allow() {
		return ErrBillingServiceUnavailable
	}

	// 鍒ゆ柇璁¤垂妯″紡
	isSubscriptionMode := group != nil && group.IsSubscriptionType() && subscription != nil

	if isSubscriptionMode {
		if err := s.checkSubscriptionEligibility(ctx, user.ID, group, subscription); err != nil {
			return err
		}
	} else {
		if err := s.checkBalanceEligibility(ctx, user.ID); err != nil {
			return err
		}
	}

	// user 脳 platform quota 浠呭湪 standard锛堜綑棰濓級妯″紡鐢熸晥锛涜闃呮ā寮忚眮鍏?
	if !isSubscriptionMode {
		if err := s.checkUserPlatformQuotaEligibility(ctx, user.ID, platform); err != nil {
			return err
		}
	}

	// Check API Key rate limits (applies to both billing modes)
	if apiKey != nil && apiKey.HasRateLimits() {
		if err := s.checkAPIKeyRateLimits(ctx, apiKey); err != nil {
			return err
		}
	}

	// RPM 闄愭祦锛氱骇鑱斿洖钀斤紙Override 鈫?Group 鈫?User锛夛紝鏀惧湪鏈€鍚庝互閬垮厤涓烘敞瀹氬け璐ョ殑璇锋眰澧炲姞璁℃暟銆?
	if err := s.checkRPM(ctx, user, group); err != nil {
		return err
	}

	return nil
}

func (s *BillingCacheService) checkUserTokenQuotaEligibility(ctx context.Context, user *User, requestedModel string) error {
	if s == nil || user == nil || !user.HasTokenLimit() || !isUserTokenQuotaModel(requestedModel) {
		return nil
	}
	if s.usageLogRepo == nil {
		return ErrBillingServiceUnavailable
	}

	now := time.Now().UTC()
	quotaDayStartedAt := timezone.StartOfQuotaDay(now)
	var usage *UserTokenUsage
	if cache, ok := s.cache.(UserTokenUsageCache); ok {
		cached, hit, err := cache.GetUserTokenUsageCache(ctx, user.ID, user.TokenQuotaStartedAt, quotaDayStartedAt)
		if err == nil && hit && cached != nil {
			usage = cached
		} else if err != nil {
			logger.LegacyPrintf("service.billing_cache", "Warning: user token usage cache read failed user=%d: %v", user.ID, err)
		}
	}

	if usage == nil {
		key := strconv.FormatInt(user.ID, 10) + ":" +
			strconv.FormatInt(user.TokenQuotaStartedAt.UnixNano(), 10) + ":" +
			strconv.FormatInt(quotaDayStartedAt.UnixNano(), 10)
		value, err, _ := s.tokenQuotaLoadSF.Do(key, func() (any, error) {
			loadCtx, cancel := context.WithTimeout(context.Background(), userTokenUsageLoadTimeout)
			defer cancel()
			return s.usageLogRepo.GetUserTokenUsage(loadCtx, user.ID, now, user.TokenQuotaStartedAt)
		})
		if err != nil {
			logger.LegacyPrintf("service.billing_cache", "ALERT: user token usage load failed user=%d: %v", user.ID, err)
			return ErrBillingServiceUnavailable.WithCause(err)
		}
		var ok bool
		usage, ok = value.(*UserTokenUsage)
		if !ok || usage == nil {
			return ErrBillingServiceUnavailable.WithCause(fmt.Errorf("unexpected user token usage type: %T", value))
		}
		if cache, ok := s.cache.(UserTokenUsageCache); ok {
			if err := cache.SetUserTokenUsageCache(ctx, user.ID, user.TokenQuotaStartedAt, quotaDayStartedAt, usage, userTokenUsageCacheTTL); err != nil {
				logger.LegacyPrintf("service.billing_cache", "Warning: user token usage cache write failed user=%d: %v", user.ID, err)
			}
		}
	}

	if user.TokenLimit1d > 0 && usage.Usage1d >= user.TokenLimit1d {
		return ErrUserToken1dQuotaExhausted
	}
	if user.TokenLimit7d > 0 && usage.Usage7d >= user.TokenLimit7d {
		return ErrUserToken7dQuotaExhausted
	}
	if user.TokenLimit30d > 0 && usage.Usage30d >= user.TokenLimit30d {
		return ErrUserToken30dQuotaExhausted
	}
	return nil
}

// IncrementUserTokenUsage updates an existing short-lived usage cache after a
// successfully persisted usage record. Cache misses are intentionally ignored;
// the next preflight rebuilds an exact rolling total from usage_logs.
func (s *BillingCacheService) IncrementUserTokenUsage(userID int64, quotaStartedAt time.Time, requestedModel string, tokens int64) {
	if s == nil || userID <= 0 || tokens <= 0 || !isUserTokenQuotaModel(requestedModel) {
		return
	}
	cache, ok := s.cache.(UserTokenUsageCache)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cacheWriteTimeout)
	defer cancel()
	quotaDayStartedAt := timezone.StartOfQuotaDay(time.Now())
	if err := cache.IncrementUserTokenUsageCache(ctx, userID, quotaStartedAt, quotaDayStartedAt, tokens); err != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: increment user token usage cache failed user=%d tokens=%d: %v", userID, tokens, err)
	}
}

func isUserTokenQuotaModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if slash := strings.LastIndex(model, "/"); slash >= 0 {
		model = strings.TrimSpace(model[slash+1:])
	}
	return strings.HasPrefix(model, "gpt-")
}

// checkRPM 鎵ц骞惰 RPM 闄愭祦锛屾墍鏈夐€傜敤鐨勯檺鍒跺悓鏃剁敓鏁堬紝浠讳竴瓒呴檺鍗虫嫆缁濓細
//
//  1. (鐢ㄦ埛, 鍒嗙粍) rpm_override       鈥?鏈€缁嗙矑搴︼細绠＄悊鍛樹负鐗瑰畾鐢ㄦ埛鍦ㄧ壒瀹氬垎缁勮瀹氱殑涓撳睘闄愰銆?//     override=0 琛ㄧず璇ョ敤鎴峰湪璇ュ垎缁勫厤妫€锛堢豢鐏級锛屼絾 user 绾у叏灞€涓婇檺浠嶇劧鐢熸晥銆?//  2. group.rpm_limit                 鈥?鍒嗙粍绾э細璇ュ垎缁勭殑缁熶竴 RPM 瀹归噺锛堜粎褰撴棤 override 鏃剁敓鏁堬級銆?//  3. user.rpm_limit                  鈥?鐢ㄦ埛绾у叏灞€纭笂闄愶細鏃犺 override/group 濡備綍閰嶇疆锛屽缁堢敓鏁堛€?//
//
// 涓庢棫鐗?绾ц仈浜掓枼"璁捐涓嶅悓锛屾柊鐗堢‘淇?user.rpm_limit 浣滀负鍏ㄥ眬澶╄姳鏉夸笉浼氳 group 鎴?override 瑕嗙洊銆?// Redis 鏁呴殰涓€寰?fail-open锛堟墦 warning锛屼笉闃诲涓氬姟锛夈€?
func (s *BillingCacheService) checkRPM(ctx context.Context, user *User, group *Group) error {
	if s == nil || s.userRPMCache == nil || user == nil {
		return nil
	}

	// 鈹€鈹€ 绗竴灞傦細鍒嗙粍绾ф鏌ワ紙override 鎴?group.rpm_limit锛?鈹€鈹€
	if group != nil {
		// 瑙ｆ瀽 override锛氫紭鍏堜粠 auth cache snapshot锛宯il 鏃跺洖閫€ DB銆?
		var override *int
		if user.UserGroupRPMOverride != nil {
			override = user.UserGroupRPMOverride
		} else if s.userGroupRateRepo != nil {
			dbOverride, err := s.userGroupRateRepo.GetRPMOverrideByUserAndGroup(ctx, user.ID, group.ID)
			if err != nil {
				logger.LegacyPrintf(
					"service.billing_cache",
					"Warning: rpm override lookup failed for user=%d group=%d: %v",
					user.ID, group.ID, err,
				)
			} else {
				override = dbOverride
			}
		}

		if override != nil {
			// override=0 鈫?璇ョ敤鎴峰湪璇ュ垎缁勫厤妫€锛堜絾 user 绾т粛浼氬湪涓嬮潰妫€鏌ワ級銆?
			if *override > 0 {
				count, incErr := s.userRPMCache.IncrementUserGroupRPM(ctx, user.ID, group.ID)
				if incErr != nil {
					logger.LegacyPrintf(
						"service.billing_cache",
						"Warning: rpm increment (override) failed for user=%d group=%d: %v",
						user.ID, group.ID, incErr,
					)
					// fail-open
				} else if count > *override {
					return ErrGroupRPMExceeded
				}
			}
			// override 鍛戒腑鍚庤烦杩?group.rpm_limit锛坥verride 鏇夸唬 group锛夛紝浣嗕笉 return鈥斺€旂户缁鏌?user 绾с€?
		} else if group.RPMLimit > 0 {
			// 鏃?override锛屾鏌?group.rpm_limit銆?
			count, err := s.userRPMCache.IncrementUserGroupRPM(ctx, user.ID, group.ID)
			if err != nil {
				logger.LegacyPrintf(
					"service.billing_cache",
					"Warning: rpm increment (group) failed for user=%d group=%d: %v",
					user.ID, group.ID, err,
				)
				// fail-open
			} else if count > group.RPMLimit {
				return ErrGroupRPMExceeded
			}
		}
	}

	// 鈹€鈹€ 绗簩灞傦細鐢ㄦ埛绾у叏灞€纭笂闄愶紙濮嬬粓鐢熸晥锛?鈹€鈹€
	if user.RPMLimit > 0 {
		count, err := s.userRPMCache.IncrementUserRPM(ctx, user.ID)
		if err != nil {
			logger.LegacyPrintf(
				"service.billing_cache",
				"Warning: rpm increment (user) failed for user=%d: %v",
				user.ID, err,
			)
			return nil // fail-open
		}
		if count > user.RPMLimit {
			return ErrUserRPMExceeded
		}
	}

	return nil
}

func (s *BillingCacheService) minimumBalanceReserve() float64 {
	if s == nil || s.cfg == nil || s.cfg.Billing.MinimumBalanceReserve <= 0 {
		return 0
	}
	return s.cfg.Billing.MinimumBalanceReserve
}

func (s *BillingCacheService) balanceBelowEligibilityThreshold(balance float64) bool {
	if balance <= 0 {
		return true
	}
	minimumReserve := s.minimumBalanceReserve()
	return minimumReserve > 0 && balance < minimumReserve
}

// checkBalanceEligibility 妫€鏌ヤ綑棰濇ā寮忚祫鏍?
func (s *BillingCacheService) checkBalanceEligibility(ctx context.Context, userID int64) error {
	balance, err := s.GetUserBalance(ctx, userID)
	if err != nil {
		if s.circuitBreaker != nil {
			s.circuitBreaker.OnFailure(err)
		}
		logger.LegacyPrintf("service.billing_cache", "ALERT: billing balance check failed for user %d: %v", userID, err)
		return ErrBillingServiceUnavailable.WithCause(err)
	}
	if s.circuitBreaker != nil {
		s.circuitBreaker.OnSuccess()
	}

	if s.balanceBelowEligibilityThreshold(balance) {
		return ErrInsufficientBalance
	}

	return nil
}

// checkSubscriptionEligibility 妫€鏌ヨ闃呮ā寮忚祫鏍?
func (s *BillingCacheService) checkSubscriptionEligibility(ctx context.Context, userID int64, group *Group, subscription *UserSubscription) error {
	// 鑾峰彇璁㈤槄缂撳瓨鏁版嵁
	subData, err := s.GetSubscriptionStatus(ctx, userID, group.ID)
	if err != nil {
		if s.circuitBreaker != nil {
			s.circuitBreaker.OnFailure(err)
		}
		logger.LegacyPrintf("service.billing_cache", "ALERT: billing subscription check failed for user %d group %d: %v", userID, group.ID, err)
		return ErrBillingServiceUnavailable.WithCause(err)
	}
	if s.circuitBreaker != nil {
		s.circuitBreaker.OnSuccess()
	}

	// 妫€鏌ヨ闃呯姸鎬?
	if subData.Status != SubscriptionStatusActive {
		return ErrSubscriptionInvalid
	}

	// 妫€鏌ユ槸鍚﹁繃鏈?
	if time.Now().After(subData.ExpiresAt) {
		return ErrSubscriptionInvalid
	}

	// 妫€鏌ラ檺棰濓紙浣跨敤浼犲叆鐨凣roup闄愰閰嶇疆锛?
	if group.HasDailyLimit() && subData.DailyUsage >= *group.DailyLimitUSD {
		return ErrDailyLimitExceeded
	}

	if group.HasWeeklyLimit() && subData.WeeklyUsage >= *group.WeeklyLimitUSD {
		return ErrWeeklyLimitExceeded
	}

	if group.HasMonthlyLimit() && subData.MonthlyUsage >= *group.MonthlyLimitUSD {
		return ErrMonthlyLimitExceeded
	}

	return nil
}

type billingCircuitBreakerState int

const (
	billingCircuitClosed billingCircuitBreakerState = iota
	billingCircuitOpen
	billingCircuitHalfOpen
)

type billingCircuitBreaker struct {
	mu                sync.Mutex
	state             billingCircuitBreakerState
	failures          int
	openedAt          time.Time
	failureThreshold  int
	resetTimeout      time.Duration
	halfOpenRequests  int
	halfOpenRemaining int
}

func newBillingCircuitBreaker(cfg config.CircuitBreakerConfig) *billingCircuitBreaker {
	if !cfg.Enabled {
		return nil
	}
	resetTimeout := time.Duration(cfg.ResetTimeoutSeconds) * time.Second
	if resetTimeout <= 0 {
		resetTimeout = 30 * time.Second
	}
	halfOpen := cfg.HalfOpenRequests
	if halfOpen <= 0 {
		halfOpen = 1
	}
	threshold := cfg.FailureThreshold
	if threshold <= 0 {
		threshold = 5
	}
	return &billingCircuitBreaker{
		state:            billingCircuitClosed,
		failureThreshold: threshold,
		resetTimeout:     resetTimeout,
		halfOpenRequests: halfOpen,
	}
}

func (b *billingCircuitBreaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case billingCircuitClosed:
		return true
	case billingCircuitOpen:
		if time.Since(b.openedAt) < b.resetTimeout {
			return false
		}
		b.state = billingCircuitHalfOpen
		b.halfOpenRemaining = b.halfOpenRequests
		logger.LegacyPrintf("service.billing_cache", "ALERT: billing circuit breaker entering half-open state")
		fallthrough
	case billingCircuitHalfOpen:
		if b.halfOpenRemaining <= 0 {
			return false
		}
		b.halfOpenRemaining--
		return true
	default:
		return false
	}
}

func (b *billingCircuitBreaker) OnFailure(err error) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case billingCircuitOpen:
		return
	case billingCircuitHalfOpen:
		b.state = billingCircuitOpen
		b.openedAt = time.Now()
		b.halfOpenRemaining = 0
		logger.LegacyPrintf("service.billing_cache", "ALERT: billing circuit breaker opened after half-open failure: %v", err)
		return
	default:
		b.failures++
		if b.failures >= b.failureThreshold {
			b.state = billingCircuitOpen
			b.openedAt = time.Now()
			b.halfOpenRemaining = 0
			logger.LegacyPrintf("service.billing_cache", "ALERT: billing circuit breaker opened after %d failures: %v", b.failures, err)
		}
	}
}

func (b *billingCircuitBreaker) OnSuccess() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	previousState := b.state
	previousFailures := b.failures

	b.state = billingCircuitClosed
	b.failures = 0
	b.halfOpenRemaining = 0

	// 鍙湁鐘舵€佺湡姝ｅ彂鐢熷彉鍖栨椂鎵嶈褰曟棩蹇?
	if previousState != billingCircuitClosed {
		logger.LegacyPrintf("service.billing_cache", "ALERT: billing circuit breaker closed (was %s)", circuitStateString(previousState))
	} else if previousFailures > 0 {
		logger.LegacyPrintf("service.billing_cache", "INFO: billing circuit breaker failures reset from %d", previousFailures)
	}
}

func circuitStateString(state billingCircuitBreakerState) string {
	switch state {
	case billingCircuitClosed:
		return "closed"
	case billingCircuitOpen:
		return "open"
	case billingCircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// checkUserPlatformQuotaEligibility 鍦?standard 妯″紡涓嬫鏌?user 脳 platform 鏃?鍛?鏈?quota銆?// 杩斿洖 nil = 鍏佽锛涜繑鍥?ErrUserPlatform{Daily/Weekly/Monthly}QuotaExhausted = 鎷掔粷锛堝甫 window_resets_at metadata锛夈€?// checkUserPlatformQuotaEligibility 妫€鏌ョ敤鎴峰湪鎸囧畾骞冲彴鐨?USD 閰嶉銆?//
// 娴佺▼锛圧edis-first / DB-fallback锛夛細
//  1. 鍏堣 Redis cache锛涜嫢鍛戒腑涓?SchemaVersion==1锛岀洿鎺ョ敤 entry 涓殑 limits 鍜?window_start 鍋氭牎楠岋紝
//     鍏嶉櫎 DB 鏌ヨ銆?//  2. cache MISS 鎴栨棫鐗?entry锛圫chemaVersion==0锛夆啋 鏌?DB 鍥炲～瀹屾暣 entry锛堝惈 limits/window_start锛夈€?//  3. Redis 鏁呴殰锛坋rr != nil锛夆啋 fail-open锛屾煡 DB 鍋氫竴娆℃€ф鏌ワ紝涓嶅洖濉€?
func (s *BillingCacheService) checkUserPlatformQuotaEligibility(
	ctx context.Context,
	userID int64,
	platform string,
) error {
	if platform == "" || s.userPlatformQuotaRepo == nil {
		return nil
	}

	// cache 鏈厤缃紙濡傜畝鍖栭儴缃?/ 鍗曟祴璺緞锛夆啋 鐩存帴璧?DB 鏌ヨ锛岄伩鍏?nil panic銆?
	// 鍏朵粬 check* 鏂规硶锛坆alance/subscription/rate-limit锛変篃鏈夌被浼煎畧鍗€?
	var (
		entry    *UserPlatformQuotaCacheEntry
		ok       bool
		cacheErr error
	)
	if s.cache != nil {
		entry, ok, cacheErr = s.cache.GetUserPlatformQuotaCache(ctx, userID, platform)
	} else {
		// 鏍囪涓?cache 鏁呴殰"鍒嗘敮锛氳烦杩?HIT 璺緞銆佷笉鍥炲～銆佽蛋 DB 涓€娆℃€ф鏌?		cacheErr = errBillingCacheUnavailable
	}

	// --- cache HIT with current schema 鈫?鐩存帴鐢?entry锛屼笉鏌?DB ---
	if cacheErr == nil && ok && entry != nil && entry.SchemaVersion == UserPlatformQuotaCacheSchemaV1 {
		now := time.Now()
		dailyUsage := entry.DailyUsageUSD
		weeklyUsage := entry.WeeklyUsageUSD
		monthlyUsage := entry.MonthlyUsageUSD
		// 鑻ョ獥鍙ｅ凡鏇存柊锛圖B 宸查噸缃絾 cache 灏氭湭澶辨晥锛?灏嗗搴?usage 娓呴浂鍐嶅仛姣旇緝,
		// 鍚屾椂璁板綍鏂扮獥鍙ｈ捣鐐圭敤浜庡悗缁埛鏂?cache entry銆?		// 鏈璇锋眰鐢ㄦ湰鍦版竻闆跺€肩户缁垽鏂?DB 灞?IncrementUsageWithReset 宸叉湁绐楀彛鑷剤鑳藉姏,
		// 鎸佷箙鍖栨暟鎹缁堟纭€?
		windowExpired := false
		newDailyStart := entry.DailyWindowStart
		newWeeklyStart := entry.WeeklyWindowStart
		newMonthlyStart := entry.MonthlyWindowStart
		if quotaWindowExpired(entry.DailyWindowStart, timezone.StartOfQuotaDay(now)) {
			dailyUsage = 0
			windowExpired = true
			dayStart := timezone.StartOfQuotaDay(now)
			newDailyStart = &dayStart
		}
		if quotaWindowExpired(entry.WeeklyWindowStart, timezone.StartOfQuotaWeek(now)) {
			weeklyUsage = 0
			windowExpired = true
			weekStart := timezone.StartOfQuotaWeek(now)
			newWeeklyStart = &weekStart
		}
		if monthlyQuotaWindowExpired(entry.MonthlyWindowStart, now) {
			monthlyUsage = 0
			windowExpired = true
			monthStart := now
			newMonthlyStart = &monthStart
		}
		// 妫€娴嬪埌浠绘剰绐楀彛杩囨湡锛氱敤 reset 鍚庣殑 entry 瑕嗙洊 Redis锛堣€岄潪 Delete锛夈€?		// 鏃у疄鐜?Delete 鍚?鏈熼棿鍒拌揪鐨?IncrUserPlatformQuotaUsage 璋冪敤璁?Lua 鐪嬪埌
		// EXISTS=0 鐩存帴 return 0,骞跺彂璇锋眰鐨?cost 姘镐箙涓㈠け,鐩村埌涓嬫 cache MISS 鍥炲～銆?		// 鏀逛负 SetCache 鍘熷瓙瑕嗙洊:key 涓嶆柇閾?Lua INCR 鍙湪鏂扮獥鍙?entry 涓婃纭疮鍔犮€?		// 瓒呮椂 50ms:瑕嗙洊姝ｅ父璺緞涓庡彲鎺ュ彈鎶栧姩;Redis 寮傚父鏃?hot path 涓嶉樆濉炶秴杩囨鍊笺€?		// 鐢?context.Background()+鐭秴鏃?閬垮厤璇锋眰 ctx 鍙栨秷瀵艰嚧鍒锋柊涓㈠け銆?		// 鏄惧紡 setCancel()(鑰岄潪 defer):缂╃煭 context 鐢熷懡鍛ㄦ湡,閬垮厤 defer 寤惰繜鍒板嚱鏁拌繑鍥炪€?		// isSentinel 鍒ゅ畾銆岃 entry 鏃犱换浣?limit銆?娑电洊涓ょ被,璺ㄧ獥鍙ｅ懡涓椂閮借烦杩?refresh:
		//   1) A3 鍥炲～鐨?sentinel(DB 鏃犺,鐭?TTL):refresh 浼氭妸鐭?TTL 璇崌绾т负 86400s,鏈夊;
		//   2) DB 鏈夎浣嗕笁 limit 鍏ㄦ湭閰嶇疆鐨勭敤鎴?TTL 86400s):refresh 绾睘鏃犳剰涔?TTL 鍗囩骇鏈韩鏃犲)銆?
		// 涓ょ被鐨?enforcement(涓嬫柟 limit!=nil 姣旇緝)閮藉洜 limit 鍏?nil 姘歌繙鏀捐,璺宠繃 refresh 鍧囨纭€?
		isSentinel := entry.DailyLimitUSD == nil && entry.WeeklyLimitUSD == nil && entry.MonthlyLimitUSD == nil
		if windowExpired && s.cache != nil && !isSentinel {
			refreshed := &UserPlatformQuotaCacheEntry{
				DailyUsageUSD:      dailyUsage,
				WeeklyUsageUSD:     weeklyUsage,
				MonthlyUsageUSD:    monthlyUsage,
				SchemaVersion:      UserPlatformQuotaCacheSchemaV1,
				DailyLimitUSD:      entry.DailyLimitUSD,
				WeeklyLimitUSD:     entry.WeeklyLimitUSD,
				MonthlyLimitUSD:    entry.MonthlyLimitUSD,
				DailyWindowStart:   newDailyStart,
				WeeklyWindowStart:  newWeeklyStart,
				MonthlyWindowStart: newMonthlyStart,
			}
			ttl := time.Duration(s.cfg.Billing.UserPlatformQuotaCacheTTLSeconds) * time.Second
			setCtx, setCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			if setErr := s.cache.SetUserPlatformQuotaCache(setCtx, userID, platform, refreshed, ttl); setErr != nil {
				logger.LegacyPrintf("service.billing_cache",
					"Warning: refresh expired user platform quota cache failed user=%d platform=%s: %v",
					userID, platform, setErr)
			}
			setCancel()
		}
		if entry.DailyLimitUSD != nil && dailyUsage >= *entry.DailyLimitUSD {
			return withWindowResetsMetadata(ErrUserPlatformDailyQuotaExhausted, nextDailyReset(now))
		}
		if entry.WeeklyLimitUSD != nil && weeklyUsage >= *entry.WeeklyLimitUSD {
			return withWindowResetsMetadata(ErrUserPlatformWeeklyQuotaExhausted, nextWeeklyReset(now))
		}
		if entry.MonthlyLimitUSD != nil && monthlyUsage >= *entry.MonthlyLimitUSD {
			return withWindowResetsMetadata(ErrUserPlatformMonthlyQuotaExhausted, nextMonthlyResetFrom(entry.MonthlyWindowStart, now))
		}
		return nil
	}

	// --- cache MISS銆佹棫鐗?entry 鎴?Redis 鏁呴殰 鈫?鏌?DB锛坰ingleflight 鍚堝苟骞跺彂鍥炴簮锛?--
	// 浣跨敤 DoChan 鑰岄潪 Do锛歛void sharing the first caller's ctx among all dedupe followers.
	// 鑻ョ涓€涓?caller 鐨?ctx 琚彇娑堬紙瀹㈡埛绔柇杩烇級锛屽悗缁?caller 涓嶅彈褰卞搷锛屼粛鐢卞悇鑷?ctx 鎺у埗瓒呮椂銆?
	sfKey := strconv.FormatInt(userID, 10) + ":" + platform
	ch := s.quotaLoadSF.DoChan(sfKey, func() (any, error) {
		// 瀛愭煡璇㈢敤 detached context + 鐭秴鏃讹紝鐙珛浜庝换浣?caller 鐨勮姹?ctx锛?		// 闃叉"绗竴涓?caller ctx 鍙栨秷"浣挎墍鏈?follower 涓€璧?fail銆?
		bgCtx, bgCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer bgCancel()
		return s.userPlatformQuotaRepo.GetByUserPlatform(bgCtx, userID, platform)
	})
	var (
		v     any
		dbErr error
	)
	select {
	case res := <-ch:
		v, dbErr = res.Val, res.Err
	case <-ctx.Done():
		// 褰撳墠 caller 鐨?ctx 琚彇娑堬細fail-open锛屼笉闃绘柇 (姝よ姹傚凡鏃犳剰涔?銆?
		logger.LegacyPrintf("service.billing_cache", "Warning: user platform quota check ctx cancelled user=%d platform=%s: %v (fail-open)", userID, platform, ctx.Err())
		return nil
	}
	if dbErr != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: load user platform quota failed user=%d platform=%s: %v (fail-open)", userID, platform, dbErr)
		return nil
	}
	rec, _ := v.(*UserPlatformQuotaRecord)
	if rec == nil {
		// 浠呭湪 cache 鍙敤涓旀湰娆?GET 鏈嚭閿欐椂鍥炲～ sentinel:Redis GET 鏁呴殰(cacheErr!=nil)
		// 鏃朵笉鍥炲～,涓庝笅鏂?line ~1201 "Redis 鏁呴殰鏃?fail-open:涓嶅洖濉? 淇濇寔涓€鑷?
		// 閬垮厤鍦?Redis 寮傚父鏈熷仛涓€娆℃敞瀹氬け璐ョ殑 SET銆?
		if s.cache != nil && cacheErr == nil {
			now := time.Now()
			startOfDay := timezone.StartOfQuotaDay(now)
			startOfWeek := timezone.StartOfQuotaWeek(now)
			sentinel := &UserPlatformQuotaCacheEntry{
				SchemaVersion:      UserPlatformQuotaCacheSchemaV1,
				DailyWindowStart:   &startOfDay,
				WeeklyWindowStart:  &startOfWeek,
				MonthlyWindowStart: &now,
				// limits 鍏?nil, usage 鍏?0(闆跺€?
			}
			sentinelTTL := time.Duration(s.cfg.Billing.UserPlatformQuotaSentinelTTLSeconds) * time.Second
			if sentinelTTL <= 0 {
				// 闃插尽:TTL<=0 鏃?Redis EXPIRE 浼氱珛鍗冲垹闄ゆ暣涓?key(瑙?billing_cache.go 鐨?pipe.Expire),
				// sentinel 涓嶆寔涔呭寲 鈫?姣忚姹傚嚮绌?DB銆傞厤缃己澶?璇厤涓?0 鏃?fallback 鍒?1h銆?
				sentinelTTL = time.Hour
			}
			setCtx, setCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			if setErr := s.cache.SetUserPlatformQuotaCache(setCtx, userID, platform, sentinel, sentinelTTL); setErr != nil {
				userPlatformQuotaSentinelSetCacheErrorTotal.Add(1)
				logger.LegacyPrintf("service.billing_cache", "Warning: set sentinel quota cache failed user=%d platform=%s: %v", userID, platform, setErr)
			}
			setCancel()
		}
		return nil
	}

	now := time.Now()
	dailyUsage := rec.DailyUsageUSD
	weeklyUsage := rec.WeeklyUsageUSD
	monthlyUsage := rec.MonthlyUsageUSD
	if quotaWindowExpired(rec.DailyWindowStart, timezone.StartOfQuotaDay(now)) {
		dailyUsage = 0
	}
	if quotaWindowExpired(rec.WeeklyWindowStart, timezone.StartOfQuotaWeek(now)) {
		weeklyUsage = 0
	}
	if monthlyQuotaWindowExpired(rec.MonthlyWindowStart, now) {
		monthlyUsage = 0
	}

	// Redis 鏁呴殰鏃?fail-open锛氫笉鍥炲～锛岀洿鎺ョ敤 DB 鏁版嵁鍋氫竴娆℃€ф鏌?
	if cacheErr != nil {
		if rec.DailyLimitUSD != nil && dailyUsage >= *rec.DailyLimitUSD {
			return withWindowResetsMetadata(ErrUserPlatformDailyQuotaExhausted, nextDailyReset(now))
		}
		if rec.WeeklyLimitUSD != nil && weeklyUsage >= *rec.WeeklyLimitUSD {
			return withWindowResetsMetadata(ErrUserPlatformWeeklyQuotaExhausted, nextWeeklyReset(now))
		}
		if rec.MonthlyLimitUSD != nil && monthlyUsage >= *rec.MonthlyLimitUSD {
			return withWindowResetsMetadata(ErrUserPlatformMonthlyQuotaExhausted, nextMonthlyResetFrom(rec.MonthlyWindowStart, now))
		}
		return nil
	}

	// cache MISS 鎴栨棫鐗?entry 鈫?鍥炲～瀹屾暣 entry锛堝惈 limits 鍜?window_start锛?
	newEntry := &UserPlatformQuotaCacheEntry{
		DailyUsageUSD:      dailyUsage,
		WeeklyUsageUSD:     weeklyUsage,
		MonthlyUsageUSD:    monthlyUsage,
		SchemaVersion:      UserPlatformQuotaCacheSchemaV1,
		DailyLimitUSD:      rec.DailyLimitUSD,
		WeeklyLimitUSD:     rec.WeeklyLimitUSD,
		MonthlyLimitUSD:    rec.MonthlyLimitUSD,
		DailyWindowStart:   rec.DailyWindowStart,
		WeeklyWindowStart:  rec.WeeklyWindowStart,
		MonthlyWindowStart: rec.MonthlyWindowStart,
	}
	if s.cache != nil {
		ttl := time.Duration(s.cfg.Billing.UserPlatformQuotaCacheTTLSeconds) * time.Second
		// 涓?HIT 杩囨湡鍥炲～璺緞锛堜笂鏂?SetCache 璋冪敤锛変繚鎸佷竴鑷达細鐢?context.Background()+50ms,
		// 閬垮厤璇锋眰 ctx 鎻愬墠鍙栨秷锛堝鎴风鏂繛/涓婃父瓒呮椂锛夊鑷?cache 鍥炲～澶辫触,
		// 璁╀笅涓€娆?preflight 浠嶇劧 MISS 骞跺嚮绌垮埌 DB锛堥珮骞跺彂涓嬪澶?DB 鍘嬪姏锛夈€?
		setCtx, setCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		if setErr := s.cache.SetUserPlatformQuotaCache(setCtx, userID, platform, newEntry, ttl); setErr != nil {
			logger.LegacyPrintf("service.billing_cache", "Warning: set user platform quota cache failed user=%d platform=%s: %v", userID, platform, setErr)
		}
		setCancel()
	}

	if rec.DailyLimitUSD != nil && dailyUsage >= *rec.DailyLimitUSD {
		return withWindowResetsMetadata(ErrUserPlatformDailyQuotaExhausted, nextDailyReset(now))
	}
	if rec.WeeklyLimitUSD != nil && weeklyUsage >= *rec.WeeklyLimitUSD {
		return withWindowResetsMetadata(ErrUserPlatformWeeklyQuotaExhausted, nextWeeklyReset(now))
	}
	if rec.MonthlyLimitUSD != nil && monthlyUsage >= *rec.MonthlyLimitUSD {
		return withWindowResetsMetadata(ErrUserPlatformMonthlyQuotaExhausted, nextMonthlyResetFrom(rec.MonthlyWindowStart, now))
	}
	return nil
}

// withWindowResetsMetadata 缁?quota error 闄勫姞 window_resets_at metadata锛圧FC3339锛夈€?
func withWindowResetsMetadata(err error, resetAt time.Time) error {
	appErr, ok := err.(*infraerrors.ApplicationError)
	if !ok || appErr == nil {
		return err
	}
	return appErr.WithMetadata(map[string]string{
		"window_resets_at": resetAt.Format(time.RFC3339),
	})
}

// nextDailyReset 璁＄畻涓嬩竴涓棩绐楀彛璧风偣锛堟鏃ュ叏灞€鏃跺尯 0 鐐癸級銆?// 蹇呴』涓?timezone.StartOfDay 鍚屽彛寰勶紝鍚﹀垯 Retry-After 浼氬亸宸€?
func nextDailyReset(now time.Time) time.Time {
	return timezone.StartOfQuotaDay(now).AddDate(0, 0, 1)
}

// nextWeeklyReset 璁＄畻涓嬩竴涓懆绐楀彛璧风偣锛堜笅鍛ㄤ竴鍏ㄥ眬鏃跺尯 0 鐐癸級銆?// 蹇呴』涓?timezone.StartOfWeek 鍚屽彛寰勶紝鍚﹀垯 Retry-After 浼氬亸宸€?
func nextWeeklyReset(now time.Time) time.Time {
	return timezone.StartOfQuotaWeek(now).AddDate(0, 0, 7)
}

// nextMonthlyResetFrom 杩斿洖 30 澶╂粴鍔ㄧ獥鍙ｇ殑涓嬫閲嶇疆鏃堕棿锛坰tart + 30d锛夈€?// start 涓?nil锛堟湭鍒濆鍖栵級鎴栧凡杩囨湡锛坣ow-start >= 30d锛屼笌 monthlyQuotaWindowExpired 鍚屽彛寰勶級鏃?// 閫€鍖栦负 now+30d锛氳繃鏈熺獥鍙ｄ細鍦ㄤ笅娆?increment 鏃堕噸缃负 now锛屼笅娆￠噸缃嵆 now+30d锛?// 鍚﹀垯鎸?start 璁＄畻浼氬緱鍒颁竴涓繃鍘荤殑鏃堕棿锛屼娇 Retry-After 钀藉洖 fallback 骞惰Е鍙戝鎴风绱у噾閲嶈瘯銆?
func nextMonthlyResetFrom(start *time.Time, now time.Time) time.Time {
	if start == nil || now.Sub(*start) >= 30*24*time.Hour {
		return now.Add(30 * 24 * time.Hour)
	}
	return start.Add(30 * 24 * time.Hour)
}

// quotaWindowExpired 鍒ゆ柇绐楀彛鏄惁宸茶繃鏈燂細start 涓?nil锛堟湭鍒濆鍖栵級鎴栧湪 currWindowStart 涔嬪墠瑙嗕负宸茶繃鏈熴€?
func quotaWindowExpired(start *time.Time, currWindowStart time.Time) bool {
	if start == nil {
		return true
	}
	return start.Before(currWindowStart)
}

// monthlyQuotaWindowExpired 鍒ゆ柇 30 澶╂粴鍔ㄦ湀搴︾獥鍙ｆ槸鍚﹀凡杩囨湡銆?// 杩囨湡鏉′欢锛歯ow - start >= 30脳24h锛堜笌璁㈤槄妯″紡 NeedsMonthlyReset 璇箟涓€鑷达級銆?// start 涓?nil 鏃惰涓哄凡杩囨湡锛堟湭鍒濆鍖栫獥鍙ｏ級銆?
func monthlyQuotaWindowExpired(start *time.Time, now time.Time) bool {
	if start == nil {
		return true
	}
	return now.Sub(*start) >= 30*24*time.Hour
}

// HasUserPlatformQuotaLimit 鍒ゆ柇璇?user脳platform 鏄惁璁句簡浠讳竴闈?nil limit銆?// 鍐欏叆鐐瑰畧鍗?鏃?limit 鐩存帴璺宠繃 Redis 鍐?+ 鑴忛泦鏍囪,娑堥櫎鏃犺皳鍐欏叆銆?// fail-safe:浠讳綍涓嶇‘瀹?simple 妯″紡闄ゅ)閮借繑鍥?true 缁存寔鍐欏叆銆?
func (s *BillingCacheService) HasUserPlatformQuotaLimit(ctx context.Context, userID int64, platform string) bool {
	if s.cfg.RunMode == config.RunModeSimple {
		return false
	}
	if s.cache == nil {
		return true
	}
	entry, ok, err := s.cache.GetUserPlatformQuotaCache(ctx, userID, platform)
	if err != nil || !ok || entry == nil {
		return true
	}
	return entry.DailyLimitUSD != nil || entry.WeeklyLimitUSD != nil || entry.MonthlyLimitUSD != nil
}
