---

### **Trigger AI: Technical Scope Document **

#### **1. Product Overview & Core Philosophy**

Trigger AI is an AI-powered sales assistant platform designed to automatically capture, analyze, and operationalize customer conversations. The platform transforms sales calls from unstructured conversations into actionable business intelligence by combining call recording, transcription, AI analysis, CRM automation, and performance insights. The core philosophy is "Actions Over Summaries"—transforming unstructured sales calls into actionable business intelligence rather than passive reporting.

#### **2. Architectural Core: Multi-Tenant CRM Agnosticism**

The heaviest lifting of the platform is built around a **multi-tenant, CRM-agnostic architecture**.

- **Tenant Isolation:** The system is designed so that each registered tenant (e.g., a real estate agency or a service business) can securely configure and manage their own isolated workspace and specific CRM connection.
- **Agnostic Integration Layer:** Instead of hardcoding logic for a single database, the Go backend will utilize an abstraction layer to standardize CRM operations (fetching leads, drafting updates, logging activities).
- **Phased Rollout:**
- **Phase 1 (Initial):** Odoo 18 will serve as the initial supported CRM integration, acting as the proof-of-concept for the integration layer.

- **Phase 2 (Future):** The architecture is explicitly designed to support seamless onboarding of new CRM systems in the future, most notably **Engaz CRM**, along with other regional or enterprise CRMs.

#### **3. High-Level Architecture & Stack**

The platform utilizes a lightweight Go web server to serve a Progressive Web App (PWA), wrapped natively for mobile devices, while delegating heavy CRM data management to tenant-configured external systems.

**Web Server (Go-centric)**

- **Language & Routing:** Go 1.25.0 (Module: `go_htmx`).

- **Dependencies:** Prefers the Go standard library (`net/http`, `html/template`, `embed`, `sync`) to remain lightweight. Battle-tested third-party packages are permitted when necessary (e.g., TUS protocol handlers, Redis clients, cloud SDKs) to avoid reinventing complex protocols.

- **Execution:** Entry point is `main.go`, running on port `:8080`.

- **Responsibilities:** Serving HTML templates, routing web requests, managing tenant configurations, and communicating with external REST APIs (like Odoo).

**Frontend (HTMX + PWA)**

- **Core Protocol:** Server-rendered HTML is the primary application protocol. No Node/Vite/React SPA architecture is utilized.

- **Interactivity:** HTMX (v2.0.4 via CDN) is used for dynamic content updates.

- **Templating & Styling:** Go's native `html/template` (housed in `templates/`). Styling is implemented via plain inline CSS in `templates/layout.html`.

- **Progressive Web App (PWA):** Features a static manifest (`/manifest.json`), service worker (`/sw.js`), and icons stored in `static/` to enable installable, offline-capable web experiences.

**File Storage & Resumable Uploads**

- **Storage Layer:** Cloudflare R2 is the primary object storage, chosen for $0 egress fees which is critical for continuous AI transcription processes.
- **Go Backend:** Implements a lightweight TUS protocol handler, binding directly to an S3/R2 Data Store. This allows chunked uploads to stream directly through the server to R2 without consuming local disk space. State management is handled natively by the TUS protocol.
- **Mobile Client:** The Kotlin application utilizes `tus-android-client` wrapped in an Android `CoroutineWorker` managed by `WorkManager`. This provides robust background streaming, capable of pausing during network drops and resuming exactly where it left off, even if the app is closed.

**Identity & Tenant Management**

- **Provider:** WorkOS is utilized for authentication and B2B multi-tenant management.
- **Tenant Isolation:** Maps WorkOS 'Organizations' 1:1 with Trigger AI Tenants, enabling the Go backend to securely route CRM traffic and isolate data (e.g., in R2 storage paths) based on the authenticated organization context.
- **Web Auth Flow:** Handled entirely server-side via the Go backend using secure, HTTP-only session cookies, aligning perfectly with the HTMX/PWA architecture and avoiding frontend token management.
- **Enterprise Readiness:** Provides out-of-the-box support for Enterprise SSO (SAML/OIDC) and Directory Sync (SCIM), critical for onboarding large-scale real estate developers or enterprise clients in future phases.

**Mobile Strategy**

- **Android Wrapper (TWA):** Built using Trusted Web Activity (TWA) via Android Gradle plugin 8.9.1. It targets the web host `trigger.exploremelon.com` using the `androidbrowserhelper` library.

- **iOS Wrapper:** A native SwiftUI WebView container utilizing WebKit. Target deployment is iOS 15.0+, managed via XcodeGen.

- **Native Recorder Application:** A separate, dedicated Android application built using **Kotlin**. It is strictly responsible for call recording, audio storage, background uploads, offline synchronization, and recording lifecycle management. The recorder application is intentionally isolated from the primary user-facing application to reduce complexity and avoid coupling mobile hardware concerns with product workflows.

**CRM Backend & Database (Odoo Ecosystem - Initial Implementation)**

- **Core Engine:** Python with Odoo 18, containerized via Docker (`odoo/docker-compose.yml`).

- **Database:** PostgreSQL 15.

- **Custom Integration:** Uses specific custom addons (`trigger_estate`, `trigger_crm_api`, `trigger_crm_demo`).

- **API:** REST endpoints are exposed via `trigger_crm_api/controllers/main.py` to allow the Go web server to fetch and mutate CRM data for tenants using Odoo.

#### **4. AI Architecture & Internal Infrastructure**

- **State & Job Orchestration:** 
  - **PostgreSQL:** Serves as the primary lightweight database for the Go server to manage system state, tenant routing configurations (WorkOS mappings), and session data.
  - **Redis:** Used to power the persistent, retry-aware background job queue (e.g., via Asynq). This ensures the asynchronous AI steps (transcription, diarization, extraction) are processed reliably without data loss during server restarts.
- **LLM & Agent Framework:** Utilizes the **Google Gen AI SDK** for robust agent orchestration, intent extraction, and drafting CRM updates.
- **Transcription Engine:** Employs **Deepgram** and/or **Whisper** for high-speed, accurate audio transcription and speaker diarization.
- **Real-Time UI Updates:** Uses **Server-Sent Events (SSE)** natively supported by HTMX (`hx-ext="sse"`) and the Go standard library. This allows the backend to broadcast job completions and dynamically update the PWA without requiring the user to refresh the page.
- **Telemetry & Observability:** Integrates **Sentry** for centralized error tracking, alongside structured JSON logging in Go. Trace IDs are generated at the mobile client and passed throughout the entire pipeline (Upload -> Queue -> AI -> CRM) to guarantee end-to-end observability.
- **Processing Path:** All AI processing operates asynchronously outside the critical customer-facing request path to ensure a fast user experience, maximum scalability, and reduced operational costs.

#### **5. Functional Product Pillars**

The scope of features is divided into four main pillars to serve Sales Representatives, Sales Managers, and Business Owners:

- **Conversation Intelligence:** Call recording, transcription, speaker separation, intent extraction, objection detection, sentiment analysis, and action item generation.

- **CRM Copilot:** Automated CRM update drafts, lead enrichment, contact management, and follow-up automation.

- **Sales Productivity:** Automated note-taking, task generation, meeting preparation, and customer history tracking.

- **Sales Intelligence:** Team dashboards, pipeline analytics, deal progression analysis, forecasting, and coaching insights.

#### **6. Success Metrics**

Technical and product success will be evaluated against:

- Reduction in manual CRM updates and administrative workload.

- Increased follow-up completion rates and opportunity conversion rates.

- Overall improvements in sales team productivity and customer revenue growth.
