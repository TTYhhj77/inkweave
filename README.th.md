# Inkweave — สตูดิโอช่วยสร้างนิยายด้วย AI Agent (Local-First)

**Inkweave** คือโปรแกรมและแอปพลิเคชันโอเพนซอร์สสำหรับช่วยแต่งนิยายด้วย AI Agent สำหรับ **PC (Windows / macOS / Linux)** และ **Android APK** พัฒนาแกนหลักด้วย **Pure Go 100%** (`modernc.org/sqlite` + `go-git/v5`) คู่กับ **Svelte 5 + CodeMirror 6 Literary Studio UI**

---

## จุดเด่นที่ออกแบบมาเพื่อนักเขียนนิยายโดยเฉพาะ

- **ระบบความจำ 4 ชั้น + ค้นหาภาษาไทยแม่นยำ**:
  - ใช้ฐานข้อมูล SQLite FTS5 คู่ (`unicode61` + `trigram`) ร่วมกับการสแกนชื่อเล่น/ฉายา ช่วยค้นหาเนื้อหานิยายภาษาไทยที่ไม่เว้นวรรคได้อย่างแม่นยำ
  - **Tier 0 (Cache-Stable)**: กฎสำนวน, POV, คำห้ามใช้ (Anti-AI-Slop), กฎโลก และ Skill วางเรียงคงที่เพื่อประหยัดค่า Prompt Caching 50–90%
  - **Tier 1 (Working Scene)**: Beat Sheet ของตอนปัจจุบัน, เนื้อหาก่อนหน้า, การ์ดวิธีพูด และ **Character State + ขอบเขตความรู้ (`UnawareOf`)** ป้องกันตัวละครหลุดรู้ความลับที่ยังไม่เกิด
  - **Tier 2 (Selective Recall)**: ค้นหา Lorebook, ความสัมพันธ์สองทิศทาง, ปมเรื่อง (`threads.yaml`), และ `timeline.yaml`
  - **Tier 3 (Rolling Summaries)**: สรุปย่อรายตอนสำหรับการเขียนนิยายยาว 100+ ตอน
  - มี **Context Inspector** กดดูได้ทุกครั้งว่าส่งข้อมูลอะไรและกี่ Token ไปหา AI

- **โหมดการทำงาน 3 ระดับ (`Eco` / `Balanced` / `Ultra`)**:
  | โหมด | งบ Token สูงสุด | รูปแบบการทำงาน | เหมาะกับ |
  | :--- | :--- | :--- | :--- |
  | **🌿 Eco (ประหยัด)** | ≤ 6,000 tokens | รอบเดียวกระชับ ใช้โมเดลประหยัด | ร่างเร็ว ประหยัดค่า API สูงสุด |
  | **⚖️ Balanced (สมดุล)** | ≤ 18,000 tokens | Prompt Cache + Tool Loop (≤ 4 รอบ) | เขียนจริงประจำวัน คุ้มค่าและแม่นยำ |
  | **🔥 Ultra (ขั้นสูงสุด)** | ≤ 48,000+ tokens | 4 ขั้นตอน: Architect → Writer → Critic (ตรวจความต่อเนื่อง & AI Slop) → Polish | ฉากสำคัญ จุดหักมุม ตรวจความซับซ้อนลึก |

- **ระบบออกแบบตัวละครมีมิติและพัฒนาการไม่เป็นเส้นตรง (Deep Character Atelier)**:
  - รองรับ **ความย้อนแย้งแกนกลาง (Core Paradox)**, **หน้ากาก vs ตัวตนจริง**, **จุดบอด (Blind Spot)**, **บาดแผล → ความเชื่อบิดเบี้ยว**, **กลไกป้องกันตัวเอง**, และ **วิธีพูดเฉพาะตัว**
  - วาดกราฟ **Non-Linear Arc** แบบอินเทอร์แอกทีฟ (รองรับรูปแบบ `Spiral`, `Oscillating`, `Fractured` พร้อมจุด **Relapse / ถดถอยกลับ**)
  - บันทึกความสัมพันธ์แบบไม่สมมาตรทั้งสองทิศทาง (`A → B` และ `B → A`) พร้อม `Public Face` และ `Hidden Agenda`

- **9 Agent Skills & 20 Story Tools + Literary Studio UI**:
  - เลือกใช้ได้ 3 ธีมสบายตา (**Paper**, **Sepia**, **Warm Dark**) ไม่มีดีไซน์ AI Slop, ระยะบรรทัดเหมาะกับสระ/วรรณยุกต์ภาษาไทย
  - **Mobile APK Layout**: มีแถบนำทาง 4 แท็บด้านล่าง, ปุ่มเรียก Quick-AI เหนือหน้ากระดาษ, และระบบแก้ Diff แบบแตะยอมรับ/ปฏิเสธ
  - **Multi-Cloud & Git Sync + Export**: ซิงก์ข้าม PC และ Android ได้ทั้งผ่าน **Git / GitHub** (`go-git/v5` ทำงานออฟไลน์บนมือถือได้จริง), **Google Drive** (OAuth 2.0 PKCE), **WebDAV** (Nextcloud/ownCloud/Synology NAS), และ **Synced Folder** (OneDrive/Dropbox/Syncthing) พร้อมตัวแก้ Conflict และระบบ Export **EPUB 3** / **Markdown**

---

## การติดตั้งและบิลด์ใช้งาน (PC & Android APK)

```powershell
# 1. คอมไพล์ UI
cd web; npm.cmd install; npm.cmd run build; cd ..

# 2. บิลด์และเปิดสตูดิโอบน PC
go build -o bin/inkweave.exe ./cmd/inkweave
./bin/inkweave.exe
```

สำหรับการสร้าง **Android APK**: ใช้สคริปต์ [`scripts/build-android.ps1`](scripts/build-android.ps1) (เมื่อมี Android SDK + `gomobile`) หรือสั่งบิลด์อัตโนมัติผ่าน GitHub Actions ใน `.github/workflows/release.yml`

---

## ลิขสิทธิ์และการนำไปใช้ (CC BY-NC-SA 4.0)

ซอฟต์แวร์ Inkweave ใช้ลิขสิทธิ์ **Creative Commons Attribution-NonCommercial-ShareAlike 4.0 International (CC BY-NC-SA 4.0)** ([LICENSE](LICENSE)):
- สามารถนำไปใช้งาน แก้ไข ดัดแปลง และแจกจ่ายต่อได้ฟรี
- **ห้ามนำไปใช้เพื่อการค้า / ห้ามจำหน่าย** ตัวแอปหรือเวอร์ชันที่ดัดแปลง
- **ShareAlike**: หากนำไปดัดแปลงและแจกจ่ายต่อ ต้องใช้ลิขสิทธิ์ **CC BY-NC-SA 4.0** แบบเดียวกัน

> **สิทธิ์ของนักเขียน**: นิยาย บทละคร ตัวละคร และไฟล์ต้นฉบับที่เขียนขึ้นด้วย Inkweave เป็นลิขสิทธิ์ของผู้เขียน **100%** สามารถนำไปตีพิมพ์หรือจำหน่ายได้ตามปกติทุกประการ
