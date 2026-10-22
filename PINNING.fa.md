# قفل سرور در RegionHop 1.24.0-pin1

این نسخهٔ سفارشی بر پایهٔ RegionHop 1.24.0 است. در پنل برای هر تونل «قفل سرور / Pin server» و «آزادکردن سرور / Unpin server» اضافه شده است.

## محدودیت مهم

این قابلیت **سرور سایفون** را قفل می‌کند؛ IP خروجی عمومی را رزرو یا تضمین نمی‌کند. آدرس نشان‌داده‌شده کنار «سرور قفل‌شده» آدرس خود سرور است و ممکن است با IP نمایش‌داده‌شده در تست خروجی فرق داشته باشد. اگر شبکهٔ سایفون خروجی همان سرور را تغییر دهد، قفل سرور جلوی آن را نمی‌گیرد. برای IP خروجی واقعاً ثابت، خروجی اختصاصی تحت کنترل خودتان لازم است.

با TargetServerEntry فقط همان سرور انتخاب می‌شود؛ اگر قطع، حذف یا مسدود شود، تونل به سرور دیگری نمی‌رود و دسترسی کاربران ممکن است قطع بماند. آزادکردن سرور انتخاب معمول سایفون را برمی‌گرداند.

## نصب روی نصب موجود

### نصب مستقیم از این Fork

ابتدا در تب Actions مخزن، Workflow با نام `Build and release pinned-server panel` باید موفق شود. اگر GitHub گردش‌کارهای Fork را غیرفعال کرده، در تب Actions گزینهٔ فعال‌کردن را بزنید و همین Workflow را با Run workflow اجرا کنید. سپس روی سروری که RegionHop از قبل نصب است:

```bash
curl -fL https://raw.githubusercontent.com/valisheidaei/regionhop/master/install-pin.sh -o /tmp/regionhop-install-pin.sh
sudo bash /tmp/regionhop-install-pin.sh
```

اسکریپت فایل مناسب معماری را از Release همین مخزن می‌گیرد، SHA256 را کنترل می‌کند، از فایل اجرایی قبلی بکاپ می‌گیرد و فقط پنل را راه‌اندازی مجدد می‌کند. اگر پنل بلافاصله از کار بیفتد، فایل اجرایی قبلی را برمی‌گرداند. اسکریپت هیچ تغییری در هسته، پورت SOCKS یا کانفیگ تونل‌ها نمی‌دهد. برای نصب اولیهٔ RegionHop از دستور اصلی استفاده کنید و سپس این ارتقای پنل را اجرا کنید.

### نصب دستی فایل اجرایی

این فایل‌ها روی سرور شما نصب نشده‌اند. تست‌ها در محیط توسعه اجرا شده‌اند؛ اتصال واقعی سایفون و systemd روی سرور شما آزمایش نشده است. ابتدا روی یک تونل کم‌مصرف امتحان کنید.

1. با `uname -m` معماری سرور را ببینید: `x86_64` یعنی فایل amd64؛ `aarch64` یعنی arm64؛ `armv7l` یعنی armv7.
2. فایل `psi-panel-linux-...` مناسب را به `/tmp/psi-panel-pin` سرور منتقل کنید.
3. از طریق SSH با دسترسی root اجرا کنید:

```bash
set -eu
test -f /opt/psi-panel/panel/psi-panel
test -f /tmp/psi-panel-pin
backup_dir="/root/regionhop-before-pin-$(date +%Y%m%d-%H%M%S)"
mkdir -m 700 "$backup_dir"
systemctl stop psi-panel
cp -a /opt/psi-panel/panel/psi-panel "$backup_dir/psi-panel"
cp -a /opt/psi-panel/data/tunnels.json "$backup_dir/tunnels.json"
cp -a /opt/psi-panel/configs "$backup_dir/configs"
if install -o root -g root -m 755 /tmp/psi-panel-pin /opt/psi-panel/panel/psi-panel; then
    systemctl start psi-panel
else
    cp -a "$backup_dir/psi-panel" /opt/psi-panel/panel/psi-panel
    systemctl start psi-panel
    exit 1
fi
systemctl is-active psi-panel
printf 'Backup: %s\n' "$backup_dir"
```

پوشهٔ پشتیبان شامل اطلاعات حساس اتصال است. مسیر چاپ‌شده را برای برگشت نگه دارید. نصب، پنل را موقتاً متوقف می‌کند؛ هنگام نصب فایل اجرایی، سرویس‌های تونل متوقف نمی‌شوند.

## استفاده

تونل را به حالت active برسانید و تست IP بگیرید. سپس «قفل سرور» را بزنید. پنل برای خواندن دیتابیس، همان تونل را کوتاه متوقف و دوباره راه‌اندازی می‌کند. اطلاعات سرور از آخرین ActiveTunnel در اجرای فعلی گرفته می‌شود، نه از IP تست اینترنت. بعد از قفل دوباره تست IP بگیرید و یک بار Restart را هم امتحان کنید. تنظیم پاسارگارد همان SOCKS محلی با پورت قبلی می‌ماند.

اگر لاگ ActiveTunnel موجود نباشد، گزینهٔ EmitDiagnosticNotices در کانفیگ تونل باید true باشد؛ یک بار Restart کنید و دوباره امتحان کنید. اگر دیتابیس Bolt استاندارد موجود نباشد یا شناسهٔ سرور مبهم باشد، عملیات خطا می‌دهد و سروری حدس زده نمی‌شود. نسخه‌های هسته با دیتابیس SQLite/Badger در این پیاده‌سازی پشتیبانی نمی‌شوند.

در حالت قفل، اطلاعات کامل سرور در رجیستری و بکاپ پنل ذخیره می‌شود؛ این فایل‌ها را محرمانه نگه دارید. تغییر Upstream و بازسازی کانفیگ، قفل را حفظ می‌کند.

آپدیت رسمی از داخل پنل یا `psictl update` این فایل اجرایی سفارشی را جایگزین می‌کند. تا زمانی که تغییرات به نسخهٔ رسمی منتقل نشده، برای حفظ قابلیت باید نسخهٔ سفارشی را نصب نگه دارید.

## برگشت

ابتدا برای تونل‌های قفل‌شده «آزادکردن سرور» را بزنید؛ سپس پنل را متوقف کنید، فایل اجرایی `psi-panel` را از پوشهٔ پشتیبان به `/opt/psi-panel/panel/psi-panel` برگردانید و پنل را راه‌اندازی کنید. اگر پنل سفارشی بالا نمی‌آید، قبل از برگشت فایل اجرایی، رجیستری و پوشهٔ configs را نیز از همان بکاپ برگردانید؛ این کار تغییرات بعد از بکاپ را کنار می‌گذارد و تونل‌های تحت‌تأثیر باید Restart شوند.

## ساخت از سورس

```bash
cd regionhop-pin/panel
go test ./...
CGO_ENABLED=0 go build -trimpath -ldflags '-s -w -X main.CurrentVersion=1.24.0-pin1' -o psi-panel .
```

تغییرات: panel/pin.go، panel/pin_test.go، مدل رجیستری، ساخت کانفیگ اتمی، دکمه‌های داشبورد، ترجمه‌های فارسی/انگلیسی و وابستگی bbolt. هستهٔ سایفون تغییر نکرده است. برای اعمال patch به مخزن اصلی از `git apply regionhop-pin.patch` استفاده کنید.

مرجع رفتار TargetServerEntry:
https://github.com/Psiphon-Labs/psiphon-tunnel-core/blob/master/psiphon/config.go
