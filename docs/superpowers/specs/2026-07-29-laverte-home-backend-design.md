# laverte-home Backend — Design

## 1. Mục đích

`laverte-home` là backend cho hệ thống đặt/thuê homestay theo giờ, qua đêm hoặc theo
ngày. Hiện quản lý 3 home, chia 2 hạng (`home`, `nest`), mỗi hạng có bảng giá riêng.
Khách đặt chỗ không cần tài khoản, thanh toán qua SePay VietQR để giữ chỗ; hệ thống
tự xác nhận booking khi nhận được tiền, đẩy lịch lên Google Calendar, nhắn Zalo cho
khách, và điều phối việc gửi mã khóa cửa lúc nhận phòng.

Dự án khởi tạo dựa trên kiến trúc của `lumen-app-backend` (clone thủ công từng phần
dùng được, không mang theo domain video/media/search của lumen).

## 2. Kiến trúc & Stack

Go module mới (`github.com/johnquangdev/laverte-home`, Go 1.25), clean architecture
theo đúng layout của lumen:

```
cmd/main.go                    # wiring, boot, auto-migrate
config/config.go               # envconfig + godotenv, Validate() ở boot
client/postgres/                # gorm + pgx client
delivery/http/                  # echo routes: booking, admin, webhook, auth, me
delivery/http/middleware/       # JWTAuth, RequireAdmin, RequireSuperAdmin, rate-limit
delivery/job/                   # cron: expire pending booking, lock-code reminder/send
usecase/                        # business logic theo domain
repository/                     # gorm queries theo domain
model/                          # gorm models
payload/, presenter/            # request/response DTO
errors/                         # apperr.Error (Code/CodeID/HTTPCode/Message/Raw)
util/checkout/                  # SePay client (tối giản, 1 provider)
util/oauth/                     # Google OAuth login — port từ lumen
util/token_store/               # Redis refresh-token store — port từ lumen
util/ratelimit/                 # Redis rate-limit — port từ lumen
util/gcalendar/                 # MỚI: Google Calendar client (service account)
util/notify/                    # MỚI: Zalo ZNS (khách) + email (admin)
migrations/                     # sql-migrate .sql, embed.FS
```

Stack giữ nguyên lumen: Echo v4, GORM + pgx (Postgres), Redis, `sql-migrate`
(auto-migrate on boot), zap logger, Air (dev hot-reload), Docker/docker-compose.
`/health` endpoint, CORS, security headers, panic-recover middleware — copy nguyên
từ `delivery/http/http.go` của lumen.

**Port nguyên từ lumen (sửa nhỏ cho domain mới):**
- `util/oauth/google.go` + `interface.go` — Google OAuth login
- `util/token_store/redis.go` — refresh-token store (rotation theo family)
- `util/ratelimit/` — Redis sliding-window limiter
- `delivery/http/middleware/` — JWTAuth, RequireAdmin, RequireSuperAdmin, RateLimitByIP/ByUser
- `model/role.go` — `RoleUser`/`RoleAdmin`/`RoleSuperAdmin`, superadmin từ env `ADMIN_USER_IDS`
- `config/config.go` pattern (struct `envconfig` + `Validate()` + `GetConfig()` singleton)
  — viết lại field-set riêng cho domain mới, không mang field video/TTS/media của lumen

**Không mang qua:** toàn bộ domain video/media/search/render/worker của lumen;
MoMo/VNPay/PayOS/SePay-PG/Router/Pool/Breaker/ProviderQuota; Plan/Subscription/Credits;
OTP xác thực số điện thoại.

## 3. Domain Model

```
Home
  id, name, category (enum: "home" | "nest"), address, description
  google_calendar_id       -- calendar Google đã share cho service account
  is_active, created_at

PricingRule                -- theo category, KHÔNG theo từng home
  id, category, rule_type (hourly | overnight | day)
  base_hours, base_price          -- hourly: N giờ đầu giá X
  extra_hour_price                -- hourly: mỗi giờ thêm giá Y (Y < base/N)
  window_start, window_end        -- overnight: khung áp dụng (vd 22:00–06:00)
  flat_price                      -- overnight/day: giá cố định
  effective_from, effective_to    -- đổi giá theo thời gian, giữ lịch sử giá cũ
  is_active

Booking
  id, home_id, customer_name, customer_phone
  start_time, end_time, booking_type (hourly | overnight | day)
  computed_price
  status (pending_payment | confirmed | cancelled | expired | completed | no_show)
  payment_id
  google_calendar_event_id
  door_lock_code                  -- admin nhập, null cho tới khi nhập
  lock_code_alert_sent_at         -- đã nhắc admin nhập mã chưa
  lock_code_sent_at               -- đã gửi mã cho khách chưa (chặn gửi trùng)
  created_by_admin_id              -- null nếu khách tự đặt, có giá trị nếu walk-in
  expires_at                       -- pending_payment tự hết hạn nếu quá giờ này
  created_at, updated_at

BlockedSlot
  id, home_id, start_time, end_time, reason, created_by_admin_id, created_at

Payment
  id, booking_id, provider ("sepay" | "cash"), amount
  status (pending | paid | expired | failed)
  qr_content, sepay_transaction_ref, paid_at, created_at

User                        -- admin/staff + khách có login Google (tuỳ chọn, để dành cho
                             -- tính năng tích điểm sau này — KHÔNG implement ở bản này)
  id, email, name, oauth_provider, oauth_id, phone, role, created_at
RefreshToken                -- y hệt lumen (token rotation theo family)
```

**Chống double-booking ở tầng DB:** Postgres exclusion constraint (extension
`btree_gist`) trên `(home_id, tstzrange(start_time, end_time))`, chỉ áp dụng khi
`status IN ('pending_payment', 'confirmed')`. Hai request đặt trùng giờ cùng lúc —
request thứ hai bị Postgres từ chối ngay ở transaction, không phải xử lý race
condition thủ công ở tầng Go.

**Tính giá (`usecase/pricing`):** nhận `(category, start_time, end_time,
booking_type)` → tra `PricingRule` đang `is_active` và trong khoảng
`effective_from`/`effective_to` tại thời điểm đặt → tính theo rule tương ứng. Admin
sửa giá qua API, không đụng code.

## 4. Luồng đặt phòng & thanh toán

1. `POST /api/v1/bookings` (public, guest) `{home_id, customer_name,
   customer_phone, start_time, end_time, booking_type}` — rate-limit theo IP và
   theo `customer_phone` (Redis, dùng lại `util/ratelimit`).
2. Usecase kiểm tra: home `is_active`, không trùng `BlockedSlot`, không trùng
   booking khác (constraint DB bắt), tính giá qua `usecase/pricing`.
3. Nếu số điện thoại này đang có booking `pending_payment` chưa hết hạn
   (`expires_at > now`) → trả về booking/QR pending đó thay vì tạo mới (chặn spam
   tạo QR trùng).
4. Tạo `Booking(status=pending_payment, expires_at=now+15p)` + `Payment(status=pending)`
   trong 1 transaction, gọi SePay tạo VietQR nội dung `LAVERTE {booking_id}` → trả
   QR cho FE.
5. Webhook `POST /api/v1/webhooks/sepay` (public, rate-limit theo IP, xác thực
   bằng HMAC-SHA256 trên `"<timestamp>.<raw body>"` kèm cửa sổ chống replay 5
   phút) → match nội dung chuyển khoản với `booking_id`, verify đúng số
   tiền, idempotent nếu đã confirmed trước đó → `Payment.status=paid`,
   `Booking.status=confirmed` → đẩy event lên Google Calendar → gửi ZNS "Đặt phòng
   thành công" cho khách.
6. Cron (`delivery/job`, `robfig/cron` như lumen) mỗi phút quét
   `Booking.status=pending_payment AND expires_at < now` → set `expired`
   (constraint tự hết hiệu lực vì status không còn nằm trong
   `pending_payment/confirmed`).

**Admin walk-in:** `POST /api/v1/admin/bookings` (JWT + RequireAdmin) — admin tạo
thẳng `status=confirmed`, `Payment` optional (`provider=cash, status=paid` nếu thu
tiền mặt tại chỗ), vẫn qua constraint DB chống trùng giờ.

**SePay client (`util/checkout/sepay.go`):**
```go
type IPaymentProvider interface {
    CreateQR(ctx context.Context, req CreateQRRequest) (*QRResult, error)
    VerifyWebhook(ctx context.Context, raw []byte, headers http.Header) (*WebhookEvent, error)
}
```
`VerifyWebhook` nhận raw bytes chứ không nhận `*http.Request`: chữ ký HMAC-SHA256
của SePay tính trên đúng body thô, marshal lại sẽ làm sai chữ ký.

Chỉ 1 implementation `SePay` (VietQR bank-transfer + webhook). Không có
Router/Pool/Breaker/ProviderQuota — bỏ hẳn so với lumen vì chỉ có 1 provider.

## 5. Google Calendar

Mỗi `Home` có `google_calendar_id` (admin điền sau khi share calendar Google cho
email service-account). `util/gcalendar.Push(ctx, home, booking)` gọi Calendar API
`events.insert` khi booking `confirmed`, lưu `event_id` vào
`Booking.google_calendar_event_id`; gọi `events.delete` khi booking bị huỷ sau khi
đã confirmed.

Xác thực bằng **Google Service Account** (JSON key), không cần ai login/consent,
không lo refresh token hết hạn. Lỗi gọi Calendar API là **best-effort, không chặn
flow thanh toán** — booking vẫn confirmed dù Calendar tạm lỗi; log lại để xử lý thủ
công (không cần queue retry ở bản đầu).

## 6. Xác nhận & mã khóa cửa

Kênh: **Zalo ZNS cho khách** (tái dùng hạ tầng ZNS đã có ở lumen, gửi theo số điện
thoại, khách không cần cài app), **email cho admin**.

- Khi booking `confirmed` (bước 5 ở mục 4): gửi ZNS "Đặt phòng thành công" cho khách.
- Admin nhập `door_lock_code` cho booking bất kỳ lúc nào trước giờ nhận phòng qua
  `PATCH /admin/bookings/:id/lock-code`.
- Cron mỗi 5 phút quét booking `confirmed`, còn ví dụ 30 phút nữa tới giờ nhận
  phòng (`start_time`), mà `door_lock_code IS NULL` và `lock_code_alert_sent_at IS
  NULL` → gửi email cảnh báo cho admin, đánh dấu đã cảnh báo.
- Khi có mã khóa: admin có thể `POST /admin/bookings/:id/send-lock-code` để gửi
  ngay (gửi sớm), hoặc cron gửi tự động đúng lúc `start_time <= now` nếu
  `lock_code_sent_at IS NULL` và mã đã có. Gửi xong đánh dấu `lock_code_sent_at`
  để không gửi trùng.

Sequence diagram kỹ thuật đầy đủ (đặt phòng → thanh toán → xác nhận → mã khóa) đã
thống nhất với anh trong phiên brainstorm, kèm bản flowchart rút gọn cho người
không rành kỹ thuật (dùng để giải thích cho chủ nhà/đối tác không kỹ thuật).

## 7. Auth & Admin API

**Auth** (port từ lumen, giữ khung, bớt trường không cần):
- `GET /api/v1/auth/google/login-url` → sinh authorize URL + lưu `state`
  (single-use, TTL 10 phút trong Redis) để chống CSRF.
- `POST /api/v1/auth/google/callback` → verify `state` → `oauth.Google.Exchange`
  → tạo/lấy `User` → phát access+refresh JWT (refresh-token rotation theo family
  qua `token_store/redis`).
- `POST /api/v1/auth/refresh`, `POST /api/v1/auth/logout`.
- Không bắt buộc cho khách đặt phòng — chỉ dùng cho admin/staff đăng nhập; giữ chỗ
  sẵn (`User.role`, `User.phone`) cho tính năng tích điểm sau này, không implement
  tích điểm ở bản này.

**Admin API** (`authed.Group("/admin", requireAdmin)`):
```
GET/POST/PUT   /admin/homes                        -- CRUD home + category
GET/POST       /admin/pricing-rules?category=...   -- xem/tạo bảng giá theo hạng
PUT            /admin/pricing-rules/:id             -- đổi giá: đóng rule cũ + tạo rule mới
GET            /admin/bookings?home_id&date=       -- danh sách booking theo home/ngày
POST           /admin/bookings                       -- tạo walk-in
PATCH          /admin/bookings/:id/cancel             -- huỷ (xoá luôn Calendar event)
PATCH          /admin/bookings/:id/complete
PATCH          /admin/bookings/:id/no-show
PATCH          /admin/bookings/:id/lock-code          -- nhập mã khóa cửa
POST           /admin/bookings/:id/send-lock-code     -- gửi mã sớm cho khách
POST/DELETE    /admin/blocked-slots                   -- chặn/mở lịch bảo trì
GET            /admin/overview?from&to                -- doanh thu, số booking theo ngày/tháng
GET/POST       /admin/admins                          -- roster admin (superadmin only)
```
Superadmin (từ `ADMIN_USER_IDS`) mới được sửa roster admin — giống cơ chế
`RequireSuperAdmin` bên lumen.

## 8. Chống spam

- Rate-limit theo IP và theo `customer_phone` (Redis) trên endpoint tạo
  booking/QR.
- Booking `pending_payment` tự hết hạn sau 15 phút (cấu hình được qua env), nhả
  chỗ tự động.
- Không tạo QR mới nếu số điện thoại đó đang có 1 booking `pending_payment` còn
  hạn — trả lại booking/QR pending sẵn có.
- Không cần OTP xác thực số điện thoại ở bản này (đã cân nhắc, quyết định
  rate-limit + auto-expire là đủ, tránh chi phí SMS/ZNS OTP).

## 9. Error handling

Copy nguyên mẫu `apperr` của lumen — `apperr.Error{Code, CodeID, HTTPCode,
Message, Raw}`, `handleErr` trung tâm ở `delivery/http/http.go` log `Raw`
server-side, trả `Code`/`Message` cho client. Thêm sentinel riêng cho domain mới:
`ErrSlotConflict` (409, khi constraint DB bắt trùng giờ), `ErrBookingExpired`,
`ErrPaymentAmountMismatch`.

## 10. Testing

- Unit test `usecase/pricing` — nhiều case biên: đúng ngưỡng giờ, overnight window
  qua nửa đêm.
- Unit test webhook SePay: đúng token, sai số tiền, đã confirmed trước đó
  (idempotency).
- Integration test tầng repository cho exclusion constraint (2 booking trùng giờ
  → 1 fail).
- Không mang test domain video/media/search của lumen.

## 11. Ngoài phạm vi (Non-goals)

- Domain video/media/search/render/worker của lumen.
- MoMo, VNPay, PayOS, SePay Payment Gateway (form checkout) — chỉ dùng SePay
  VietQR bank-transfer.
- Router/Pool/Breaker/ProviderQuota đa-provider.
- Plan/Subscription/Credits.
- OTP xác thực số điện thoại khi đặt phòng.
- Tính năng tích điểm cho khách có tài khoản Google — chỉ giữ chỗ trong model,
  chưa implement.
