# Nodus Health MVP Charter

**Status:** Authoritative

**Baseline date:** 21 August 2026

**Applies to:** `nodus-be` and `nodus-fe`

## 1. Purpose

Nodus Health's MVP is a configurable, multi-tenant hospital information
management system for Kenyan KEPH Level 2, Level 3, and Level 4 facilities. It
must support a real hospital pilot, produce the records required for safe care
and national reporting, and complete the relevant DHA, HIE, KHIS, and SHA
workflows in the official test environments.

The MVP is complete when the product passes the pilot exit criteria in this
document. Formal DHA certification, production HIE credentials, and approval of
an individual facility are launch gates controlled by external authorities;
they are not software-completion criteria and must not be represented as such.

This charter is the single source of truth for MVP scope. The frontend clinical
roadmap may sequence this scope but may not add to or remove from it.

## 2. Facility profiles and users

One product serves all three facility levels. Each tenant enables only the
departments, service points, reports, claim paths, and permissions it operates.
Disabling a module hides its workflow and prevents use of its endpoints; it
does not create a separate product edition.

| Profile | MVP capability |
| --- | --- |
| Level 2 | Registration, outpatient care, basic dispensing/inventory, referrals, billing, required reporting, and applicable national integrations |
| Level 3 | Level 2 plus maternity/MCH, basic laboratory, observation or short-stay care, and the facility's configured services |
| Level 4 | Full primary-care-hospital profile: outpatient, emergency, inpatient, maternity, surgery, diagnostics, pharmacy, referrals, mortuary, billing, reporting, and national integrations |

Primary users are registration and front-desk staff, health-records officers,
nurses and midwives, clinicians, pharmacists, laboratory and imaging staff,
theatre staff, mortuary staff, cashiers and billing/claims officers, facility
administrators, auditors, and system administrators. A patient's representative
or guardian participates in consent where required but does not receive a
patient portal in this MVP.

## 3. Definition of the pilot MVP

The pilot MVP must:

1. Run the enabled workflows below end to end against persistent backend data;
   local demo stores and seeded UI prototypes do not count.
2. Support representative Level 2, Level 3, and Level 4 tenant configurations
   without code forks.
3. Complete the current PHC, SHIF outpatient/inpatient, and ECCIF emergency
   integration paths in the official DHA/HIE sandbox, including failure,
   callback, and reconciliation paths.
4. Generate and validate the MOH/KHIS reports applicable to the pilot facility,
   with electronic submission where official access is provided and approved
   export/manual submission otherwise.
5. Pass tenant-isolation, permission, privacy, security, backup/restore,
   downtime/back-entry, clinical safety, financial reconciliation, and user
   acceptance testing.
6. Produce the policies and evidence listed in
   [compliance-traceability.md](compliance-traceability.md).

## 4. Locked feature baseline

### 4.1 Platform, facility, and workforce administration

- Multi-tenant organizations with Level 2/3/4 feature profiles, departments,
  service points, wards, rooms, beds, queues, stock locations, theatres,
  diagnostic locations, and mortuary storage locations.
- Staff onboarding and lifecycle, professional and regulatory identifiers,
  roles, least-privilege permissions, MFA, session management, access reviews,
  and explicit suspension/deactivation.
- Configurable clinical forms, warning thresholds, service and tariff
  catalogues, medication and allergen catalogues, national terminology
  mappings, and versioned report definitions.
- Immutable security and business audit history for access, exports,
  corrections, cancellations, overrides, approvals, transfers, results,
  dispensing, financial actions, claims, and national transmissions.
- Backup, restore, disaster recovery, monitoring, incident evidence, retention,
  archival, and controlled downtime/back-entry procedures.

### 4.2 Patient administration and flow

- Client Registry search and linkage; local patient registration; national and
  local identifiers; duplicate detection, correction, and audited merge.
- Demographics, contacts, next of kin, representatives/guardians, deceased
  status, communication preferences, and consent/refusal history.
- Basic appointments and arrival management, walk-in check-in, queues, visit
  and admission history, patient clinical timeline, document access/export
  requests, and closed-loop referrals.
- Consent using the current DHA flows, including OTP, biometric hand-off,
  minors/dependants, refusal, incapacity, emergency access, token refresh, and
  visit closure. Local administrative consent is not a substitute for HIE or
  claims consent.

### 4.3 Shared clinical record

- Outpatient triage and consultation; allergies; configurable observations and
  alerts; structured and narrative notes; procedures; diagnoses; prescriptions;
  orders; results; authorship; corrections; and visit summaries.
- ICD-11 diagnosis coding and the national terminology service for diagnoses,
  medicines, laboratory tests, and procedures. Local display terms retain their
  mapping to the nationally recognized code and terminology version.
- Emergency care with identified or unidentified patients, rapid registration,
  arrival mode, acuity, stabilization, resuscitation documentation, emergency
  overrides, later identity reconciliation, disposition, and admission.
- Admission requests, waiting for a bed, atomic bed assignment and transfer,
  ward census, nursing notes, medication administration, repeated observations,
  care plans, ward rounds, discharge readiness, discharge summary, disposition,
  and bed release.

### 4.4 Level 4 departmental workflows

- **Laboratory:** catalogue and reference ranges, order, specimen labels,
  collection, receipt, rejection, processing, structured/narrative result,
  abnormal and critical flags, verification, amendment, acknowledgement, and
  result history.
- **Pharmacy and clinical inventory:** formulary, prescription review,
  substitutions, allergy override, dispensing, partial fill, return and
  cancellation; stock locations, batches, expiry, receipts, adjustments,
  transfers, issues, immutable movements, stocktake, reorder and expiry alerts.
- **Imaging:** catalogue, order, scheduling/worklist, performance, report,
  verification, amendment, cancellation, and report history. Images remain in
  an external or future PACS; the MVP is report-based.
- **Theatre:** procedure scheduling, team assignment, safety and preoperative
  checks, anaesthetic record, operative note, outcomes, specimens, recovery,
  consumable usage, cancellation, and audited correction.
- **Maternity and MCH:** pregnancy episode, ANC, risk assessment, labour
  admission, partograph, delivery, maternal outcome, newborn linkage and
  outcome, postnatal care, family planning, growth monitoring, immunization
  schedule, dose/batch traceability, contraindication, deferral, and catch-up.
- **Transfusion:** request, blood group and crossmatch, unit reservation/issue,
  administration observations, reaction, return/disposal, and traceability.
  Donor recruitment, collection, component production, and regional blood-bank
  operations are excluded.
- **Basic rehabilitation:** assessment, plan, session note, progress, outcome,
  and referral using the shared encounter/order model rather than separate
  specialty systems.

### 4.5 Disease programmes and public health

- Structured registers and required report fields for HIV, tuberculosis,
  malaria, nutrition, and other programmes enabled for the facility.
- Routine programme care uses the shared visit, encounter, diagnosis, order,
  medication, observation, and referral records. Dedicated longitudinal
  treatment engines and programme-specific decision support are excluded.
- Reportable-condition alerts, morbidity/mortality coding, data-quality review,
  correction, approval, submission status, and an evidence trail.

### 4.6 Referral and mortuary

- Inbound and outbound referrals linked to the patient's care context, with
  draft, sent, received, acknowledged, accepted/declined, attended,
  return-of-care, and closed states; printable/versioned referral documents.
- Deceased-person identification and reconciliation, death notification and
  certification data, ICD-11 cause of death, body receipt and condition,
  storage allocation, movement history, property register, viewing, transfer,
  and release authorization with recipient verification and chain of custody.
- Post-mortem request, authorization, scheduling, examination/report,
  cause-of-death update, specimen chain of custody, amendment, completion, and
  controlled disclosure. Mortuary services and post-mortem fees flow into the
  common billing and cashier module.

### 4.7 Billing, cashier, and payer management

- Versioned service, product, package, bed-day, procedure, and professional-fee
  tariffs, with effective dates and payer-specific rules.
- Automatic and authorized manual charge capture, estimates, invoices, credit
  notes, deposits, discounts and waivers with approval, receipts, refunds,
  payer splits, patient/corporate credit accounts, cashier shifts, till
  balancing, and daily reconciliation.
- Every claim or invoice line is traceable to the delivered service, order,
  dispense, stock movement, admission day, or authorized manual adjustment.
- Full accounting—including general ledger, accounts payable, budgeting, tax
  accounting, and financial statements—is outside the MVP.

### 4.8 DHA, HIE, SHA, and KHIS

- OAuth client credentials and safe token handling; Client, Facility, and Health
  Worker Registry lookup/validation; registry identifiers stored alongside
  local identifiers.
- National terminology lookup and mapping, and FHIR Shared Health Record
  consent, open-visit discovery, read, collection-Bundle write, token refresh,
  security labels, and visit closure.
- Durable, idempotent outbound delivery with retries, dead-letter visibility,
  operator reprocessing, request/response metadata, reconciliation, and no
  secrets or sensitive payloads in application logs.
- SHA eligibility, sub-benefits and interventions; PHC capitation; SHIF
  outpatient and inpatient; ECCIF identified and unidentified emergency cases;
  OTP/biometric authorization; normal/elective and specialized
  preauthorizations; interventions; diagnoses; billing lines; attachments;
  preview; submission/discharge; closure; callbacks; corrections,
  resubmissions, status, utilization, and remittance reconciliation.
- Generic structured capture and attachments support surgical, imaging,
  oncology, renal, and optical preauthorizations required by UAT. This does not
  bring full oncology, renal, or optical clinical systems into scope.
- Versioned MOH/KHIS Aggregate and Tracker definitions, derivation from source
  records, completeness/consistency validation, HRIO review and approval,
  electronic submission when access is available, export fallback, receipt,
  error correction, resubmission, and reconciliation.

## 5. Explicit exclusions

The following are not MVP features:

- General ledger, accounts payable/receivable ERP, budgeting, payroll, staff
  rostering, recruitment, procurement, fixed assets, fleet management, catering,
  laundry, maintenance, and comprehensive warehouse management.
- PACS/DICOM image storage or viewing, laboratory-analyser interfaces, medical
  device integration, donor-centre blood operations, and full central sterile
  services inventory.
- Dedicated ICU/HDU, oncology, chemotherapy, radiotherapy, dialysis, dental,
  optical, ENT, mental-health, or other specialist departmental systems. Such
  care may still be documented using shared encounters and referrals.
- Telemedicine, patient portal/mobile application, automated appointment
  reminders, ambulance dispatch/GPS, arbitrary report builders, research data
  workspaces, AI documentation, and clinical decision-support engines beyond
  configured warnings.
- Offline-first browser writes, a facility-server synchronization topology, or
  multi-country regulatory support.

An excluded item cannot be added because a screen would be convenient or a
pilot user requests it informally. It requires the change process below.

## 6. Architectural boundaries

- Reuse the existing Go domain/service/repository/handler structure, PostgreSQL
  persistence and row-level tenant isolation, React application structure,
  RBAC, audit facilities, and transactional PostgreSQL outbox. Do not introduce
  a new architectural pattern, state-management approach, library, or database
  merely to implement this charter.
- The cloud-hosted service is authoritative. During connectivity loss, the
  facility uses controlled downtime forms and later audited back-entry.
  Outbound national transactions queue and retry after connectivity returns.
- Binary claim evidence, signed reports, referral documents, and mortuary
  documents require encrypted, access-controlled object storage with metadata
  in PostgreSQL. This is a required new persistence capability; its provider,
  lifecycle, malware scanning, encryption, backup, and deletion design must be
  approved in a dedicated technical design before implementation.
- National integrations are versioned external contracts. Adaptation to an
  official schema, code set, tariff, report definition, or endpoint change is
  compliance maintenance, not expansion of product scope.

## 7. Delivery guardrails and change control

- Work ships as end-to-end backend/frontend slices. A prototype that writes to
  browser storage or uses fixtures remains `prototype`, not `implemented`.
- Each slice includes permissions, audit, tenant isolation, validation,
  concurrency handling, failure states, tests, operational documentation, and
  applicable reporting/integration mappings.
- The feature-status inventory below is maintained whenever a slice changes
  state. More detailed symbol-level inventories belong in phase specifications,
  not in this charter.
- A proposed scope change must state the problem, affected facility level,
  regulatory or pilot evidence, effect on exclusions and pilot date, and the
  feature being removed or deferred to preserve the MVP boundary. The product
  owner must approve a dated charter revision before implementation.
- Regulatory sources are reviewed quarterly and immediately before DHA UAT or
  certification. Changes to obligations update the compliance matrix and may
  require a charter revision if they add a genuinely new capability.

## 8. Current feature-status inventory

| Capability | Status at baseline | MVP action |
| --- | --- | --- |
| Multi-tenancy, RLS, organization onboarding | Implemented | Harden and produce compliance evidence |
| Authentication, MFA, sessions, staff, roles | Implemented/in progress | Complete security controls and access-review evidence |
| Audit logging | Implemented foundation | Extend to all sensitive reads, exports, clinical, finance, and integration actions |
| Patient registration and management | Implemented foundation | Add national identity, representatives, appointments, access/export workflow |
| Facility resources and queues | Implemented foundation | Extend configuration and production workspaces |
| Outpatient check-in, triage, consultation | Implemented foundation | Remove remaining mock dependencies and stabilize end to end |
| Clinical catalogues, templates, ICD-11, orders | Implemented foundation | Bind to national terminology and departmental fulfillment |
| Outpatient dashboards and service hub | Prototype/mock in places | Replace local stores with aggregate and order APIs |
| Admission, bed board, ward workspace | Frontend prototype only | Implement persistent backend and replace demo store |
| Emergency, laboratory, pharmacy fulfillment, imaging, theatre, maternity/MCH | Required but absent or partial prototype | Deliver as scoped vertical slices |
| Inventory, transfusion, rehabilitation, referral, mortuary | Required but absent | Deliver as scoped vertical slices |
| Billing, cashier, SHA claims, remittance | Required; prototype hints only | Implement common revenue and claims spine |
| DHA registries, consent, SHR, terminology | Required but absent | Implement and pass sandbox scenarios |
| MOH/KHIS reporting | Required but absent | Implement versioned reporting and submission/export workflow |

## 9. Pilot exit criteria

- A representative patient can move through every enabled Level 4 department
  without duplicate identity, lost clinical context, untraceable charge, or
  unaudited override.
- Emergency admission, inpatient transfer and discharge, surgery, maternity and
  newborn linkage, departmental orders/results/dispensing, referral, death and
  mortuary release all complete with consistent state and histories.
- Cash, credit, split-payer and SHA accounts reconcile from service delivery to
  invoice or claim, receipt/remittance, refund or adjustment.
- The current official DHA UAT scenarios for PHC, SHIF, and ECCIF pass in the
  sandbox, including consent variants, retries, callbacks, rejection,
  correction, resubmission, and reconciliation.
- Applicable KHIS reports reproduce agreed test fixtures from patient-level
  source records, pass validation and approval, and can be submitted or
  exported with a receipt and correction trail.
- Security, tenant-isolation, authorization, sensitive access logging, restore,
  recovery, downtime/back-entry, 20-year retention configuration, data export,
  and incident-response exercises pass and produce reviewable evidence.
- Clinical, operational, finance, HRIO, records, privacy, and facility leadership
  representatives sign off user acceptance for their enabled workflows.

## 10. Authoritative baseline sources

- [Health Infrastructure Norms and Standards 2017](https://api.kmhfr.health.go.ke/media/Health_Infrastructure_Norms_and_Standards_2017.pdf)
- [Clinical Guidelines for Level 4–6 Hospitals, Volume 3 (2024)](https://health.go.ke/sites/default/files/2025-04/Clinical%20Guidelines%20for%20Management%20Level%204%20Primary%20Care%20Hospitals%20Level%205%20Secondary%20Hospitals%20Level%206%20Tertiary%20Hospitals%20%E2%80%93%20National%20Hospitals.pdf)
- [Digital Health Act, 2023](https://new.kenyalaw.org/akn/ke/act/2023/15/eng%402023-11-24)
- [Digital Health (Health Information Management Procedures) Regulations, 2025](https://new.kenyalaw.org/akn/ke/act/ln/2025/76/eng%402025-04-11)
- [Digital Health (Data Exchange Component) Regulations, 2025](https://new.kenyalaw.org/akn/ke/act/ln/2025/77/eng%402025-04-11)
- [DHA Digital Health Certification](https://certification.dha.go.ke/)
- [DHA HIE API documentation](https://hie-docs.dha.go.ke/)
- [DHA HIE integration scenarios](https://hie-docs.dha.go.ke/docs/scenarios/overview)
- [DHA HIE go-live process](https://hie-docs.dha.go.ke/docs/goLive)
- [Kenya National Health Terminology Service](https://nhts.dha.go.ke/)
- [KHIS Aggregate](https://khis.health.go.ke/)

Where an API specification, reporting tool, terminology release, tariff, or
certification checklist differs from this dated baseline, the current official
artifact controls and the compliance matrix must record the change.
