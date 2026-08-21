# Nodus Health MVP Compliance Traceability

**Baseline date:** 21 August 2026

**Scope authority:** [MVP Charter](mvp-scope.md)

## 1. Purpose and use

This matrix turns regulatory and national-integration obligations into product
behavior and testable evidence. It is a delivery and certification-readiness
tool, not legal advice or a claim of certification.

For each pilot facility, the data controller and processor roles, applicable
reports, enabled services, registry identifiers, sandbox/production status, and
named evidence owner must be filled in before go-live. Evidence must identify
the application version, environment, test date, reviewer, and result.

## 2. Legal and certification controls

| Obligation | Required product or operational control | Acceptance evidence | Owner | Source |
| --- | --- | --- | --- | --- |
| Use a DHA-certified solution for healthcare and System access | Maintain certification scope, self-attestation, application evidence, non-conformance log, and certificate/renewal status; block claims of certification until issued | Completed self-attestation and application pack; certificate recorded when issued | Product owner / compliance lead | HIM Regulations 36–42 |
| Register as data controller/processor and notify DHA | Record legal entities, ODPC registrations, processing roles, notification date, and facility data-controller details | ODPC evidence and DHA notification acknowledgement | Data protection officer (DPO) | HIM Regulations 4 and 38 |
| Complete a DPIA and security assessment | Maintain DPIA, threat/risk register, privacy/security policy, cyber-security assessment, backup/recovery policy, system manual, and requirements specification | Approved current documents tied to release | DPO / security lead | HIM Regulation 38 |
| Protect health data throughout its lifecycle | TLS, encryption at rest, secrets management, least privilege, MFA, secure sessions, environment separation, patching, vulnerability management, and protected backups | Configuration review, penetration/vulnerability report, key-rotation and restore evidence | Security lead | Digital Health Act 24 and 61; HIM Regulation 13 |
| Personalized authentication and role-based access | Unique accounts, no shared clinical users, RBAC, narrow privileged permissions, joiner/mover/leaver workflow, periodic access review | Permission tests, access-review report, deactivation drill | Facility administrator / security lead | Digital Health Act 24 |
| Audit all activity and health-data requests | Append-only actor, tenant, patient/resource, action, purpose/reason, result, time, request ID and source metadata for reads and writes; protect audit access | Audit test set covering view, export, correction, override, disclosure, finance and integrations | Security lead / records officer | Digital Health Act 24; HIM Regulations 17, 19 and 26 |
| Retain System health data for at least 20 years | Configurable retention/archival policy; prevent ordinary deletion; legal disposition and archival workflow; preserve provenance and readability | Retention configuration and archival/restore test | Records officer / DPO | Digital Health Act 25 |
| Support lawful access and disclosure | Verify requester, authority, guardian/representative and purpose; record consent/data-sharing agreement; controlled export and delivery; deadline tracking | Access-request and disclosure test cases with full log | Records officer / DPO | HIM Regulations 17, 19, 25 and 26 |
| Maintain data quality | Required fields, controlled codes, validation, duplicate management, provenance, correction rather than silent overwrite, quality work queues and sign-off | Data-quality dashboard and sampled record audit | HRIO / clinical lead | HIM Regulation 39; Data Exchange Regulations 12–20 |
| Backup and recovery | Encrypted backups, separated access, defined RPO/RTO, restore tests, continuity plan, incident escalation and evidence retention | Successful scheduled restore and continuity exercise | Operations / security lead | Digital Health Act 24; HIM Regulation 38 |
| Report public-health events and required minimum data | Versioned report definitions, alert rules, completeness checks, HRIO approval, submission/export receipts and resubmission history | Fixture-to-report reconciliation and submission/export receipt | HRIO / public-health lead | Data Exchange Regulation 20; HIM Regulation 39 |

## 3. National identity, terminology, consent, and SHR

| Obligation | Required product behavior | Acceptance evidence | Owner | Source |
| --- | --- | --- | --- | --- |
| Use national registry identity | Search/link Client Registry, validate Facility Registry code and Health Worker Registry practitioner ID, retain registry provenance and reconcile local duplicates | Positive, no-match, multiple-match, corrected identity and unidentified-emergency tests | Records officer / integration lead | Data Exchange Regulations 13–16; HIE Registries API |
| Use the national data dictionary and coded data | Query/synchronize authorized terminology; store code, system, version and display; map local catalogues; prevent free text where a required controlled code exists | ICD-11, LOINC, ICHI and product mapping fixtures as applicable | Clinical informatics lead | Data Exchange Regulation 12; Terminology Service API |
| Obtain encounter-specific consent | Check open visit, create consent, handle OTP/biometric flow, representative/minor, incapacity, emergency, refusal, resend, status, expiry, refresh and close; never reuse a token across patients/visits | End-to-end consent matrix including refusal and closure | DPO / integration lead | HIE Consent and SHR APIs |
| Respect sensitivity/security labels | Synchronize label catalogue, apply required labels to outgoing resources, restrict display/export, and audit access to sensitive categories | Normal/restricted and HIV/mental-health/substance-use label tests | DPO / clinical lead | SHR security-label endpoints |
| Write and read the national SHR | Produce valid FHIR collection Bundles with EpisodeOfCare/Encounter references; read only with valid consent and practitioner identity; paginate and preserve provenance | DHA sandbox bundle validation and read-back comparison | Integration lead | Data Exchange Regulation 19; SHR API |
| Transmit encounters promptly | Queue SHR writes so normal encounters are transmitted within 24 hours; record approved exceptional offline delay and transmit within seven days; expose overdue work | Clock, outage, retry, overdue and reconciliation tests | HRIO / operations | Data Exchange Regulation 19 |
| Keep transmission metadata | Record external operation, correlation/idempotency identifiers, timestamps, status, attempts, sanitized error and acknowledgement without logging sensitive payloads or credentials | Transmission-ledger inspection and log-redaction tests | Integration / security lead | Data Exchange Regulation 19 |

## 4. SHA claims and preauthorizations

The current official UAT gate spans all three funds. Each workflow must be
demonstrated against the DHA/HIE test environment and retained as regression
evidence.

| Flow | Minimum demonstrated behavior | Evidence |
| --- | --- | --- |
| Shared prerequisites | OAuth token, patient registry search, eligibility, sub-benefits, intervention coverage and interpretation of fund/access/payment/preauthorization flags | Sanitized request/response trace and UI recording |
| PHC fund | Capitation visit, consent, add/retire/switch intervention, diagnosis and billing, submit, callback/status, correction/resubmission where applicable | Successful end-to-end scenario and reconciliation |
| SHIF outpatient | OTP or biometric consent, outpatient visit, normal/elective preauthorization as indicated, interventions, diagnoses, lines, evidence, preview and submit | Successful normal and elective scenarios |
| SHIF inpatient | Admission-linked visit, applicable per-diem or fee-for-service path, normal/elective/specialized preauthorization, interim changes, discharge consent and claim dispatch | Successful per-diem and fee-for-service discharge scenarios |
| ECCIF identified emergency | Emergency case, protocol, doctor consent, interventions/evidence, submission within the required window and status | Successful timed scenario |
| ECCIF unidentified emergency | Temporary emergency identity, care/claim creation, later registry identification and reconciliation, submission and status | Successful unidentified-to-identified scenario |
| Specialized preauthorization | Generic structured capture and attachments for surgical, imaging, oncology, renal and optical UAT cases without implying full specialty clinical modules | Accepted representative requests and cancellation |
| Claim lifecycle | Add/edit/remove line, diagnosis, doctor and attachment; set coverage; preview; close/discard; rejection correction; resubmit; callback deduplication | State-transition and idempotency tests |
| Remittance | Retrieve remittance and paid claims, match to submitted lines/invoices, record variances and operator resolution | Balanced and variance reconciliation fixtures |

Official source: [DHA HIE integration scenarios](https://hie-docs.dha.go.ke/docs/scenarios/overview),
[Claims and Preauths documentation](https://hie-docs.dha.go.ke/docs/claims/getting-started/introduction),
and [go-live UAT guidance](https://hie-docs.dha.go.ke/docs/goLive).

## 5. MOH/KHIS reporting controls

The HRIO must inventory the reports applicable to the pilot facility and record
their official name/code, programme, frequency, data elements, disaggregation,
KHIS data-set/program identifier, source/version, effective date, due date, and
submission method. A static form list in code is not authoritative.

The MVP reporting engine must provide:

- Versioned definitions and effective dating without changing historical
  submissions.
- Traceability from every aggregate or tracker value to eligible source records
  and the exact derivation rule.
- Required-field, code, range, internal consistency, duplicate, completeness,
  and period/facility validation.
- Draft, reviewed, approved, submitted, accepted/rejected, corrected, and
  resubmitted states with separation of preparer and approver where configured.
- API submission through the approved KHIS/DHA route when credentials and an
  official interface are available; otherwise an approved CSV/PDF/manual-entry
  artifact with checksum, receipt, and reconciliation.
- Reportable-condition alerts and late/incomplete submission work queues.
- At minimum, coverage for enabled service domains: outpatient, inpatient
  morbidity/mortality, maternity and perinatal care, MCH/immunization, family
  planning, laboratory, pharmacy/commodities, theatre, referrals, nutrition,
  HIV, TB, malaria, mortality/cause of death, and any currently mandated oxygen
  reporting.

Acceptance evidence includes official sample periods reconciled independently
by the HRIO, validation failures, correction/resubmission, authorization tests,
and a submission or export receipt.

## 6. Cross-cutting test catalogue

- Tenant isolation for every new table, repository query, background job,
  document key, report, export, callback and integration credential.
- Permission denial and sensitive-data masking for every role and workspace.
- Concurrent check-in, bed allocation/transfer, stock issue, dispensing,
  cashier action, report approval, claim update and callback.
- Idempotent retries after timeouts and ambiguous upstream responses; duplicate
  callback delivery; poison-message visibility and authorized reprocessing.
- Audit completeness on success, rejection, override and correction without
  secrets, OTPs, biometric data, tokens, clinical payloads, or document bodies
  leaking to normal logs.
- Backup restoration, document recovery, expired/rotated credentials, national
  service outage, controlled back-entry, and reconciliation after recovery.
- Twenty-year retention configuration, archive readability, legal disclosure,
  and prevention of unauthorized hard deletion.

## 7. Evidence register template

Each control or integration scenario receives an entry with:

| Field | Required value |
| --- | --- |
| Control/scenario ID | Stable identifier linked to this matrix |
| Requirement source | Document, section, version and effective date |
| Application build | Immutable backend and frontend revision/image |
| Environment | Test, UAT, staging or production; no live credentials in the record |
| Facility/profile | Tenant and Level 2/3/4 profile used |
| Preconditions/data | Sanitized fixture identifiers and configuration version |
| Result | Pass, fail, not applicable, or externally blocked, with reason |
| Evidence | Test report, sanitized trace, screenshot/video, receipt or signed review |
| Reviewer | Named role and review date |
| Remediation | Issue reference, owner, due date and retest evidence |

## 8. Source register and review cadence

- [Digital Health Act, 2023](https://new.kenyalaw.org/akn/ke/act/2023/15/eng%402023-11-24)
- [Health Information Management Procedures Regulations, 2025](https://new.kenyalaw.org/akn/ke/act/ln/2025/76/eng%402025-04-11)
- [Data Exchange Component Regulations, 2025](https://new.kenyalaw.org/akn/ke/act/ln/2025/77/eng%402025-04-11)
- [DHA Certification programme](https://certification.dha.go.ke/)
- [DHA HIE documentation and API catalogue](https://hie-docs.dha.go.ke/)
- [Kenya National Health Terminology Service](https://nhts.dha.go.ke/)
- [KHIS Aggregate](https://khis.health.go.ke/)

The compliance lead reviews official sources quarterly, at the start of every
integration phase, and immediately before UAT/certification. A changed external
schema or reporting definition must update this matrix, its fixtures, and the
evidence set before release.
