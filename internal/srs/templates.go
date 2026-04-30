package srs

// goTemplate wrappers for the 5 SRS pipeline prompt templates.
// Ported from the Python project's Jinja2 templates.

// tmplFRNFRExtraction is the Stage 1 prompt: extract FR and NFR requirements from data.
const tmplFRNFRExtraction = `You are a Senior Requirements Engineer analyzing user feedback data.

Your task is to extract SOFTWARE REQUIREMENTS based on the data below.

DATA:
{{.Data}}

INSTRUCTIONS:
1. Read the data carefully, focusing on what users WANT, NEED, LACK, or COMPLAIN about.
2. Extract requirements and classify each as:
   - FR  (Functional Requirement)  — something the system must DO
   - NFR (Non-Functional Requirement) — HOW WELL the system works
3. For each requirement write:
   - ID:          FR-001, FR-002 ... or NFR-001, NFR-002 ...
   - Type:        FR or NFR
   - Category:    For FR: appropriate functional category
                  For NFR: Performance | Security | Usability | Reliability | Scalability
   - Statement:   "The system shall ..."
   - Source:      Short evidence from the data
   - Rationale:   Why this requirement matters

OUTPUT FORMAT — respond ONLY with valid JSON, no markdown fences:
{
  "functional_requirements": [
    {
      "id": "FR-001",
      "type": "FR",
      "category": "<category>",
      "statement": "The system shall ...",
      "source": "<evidence>",
      "rationale": "<why this matters>"
    }
  ],
  "non_functional_requirements": [
    {
      "id": "NFR-001",
      "type": "NFR",
      "category": "<category>",
      "statement": "The system shall ...",
      "source": "<evidence>",
      "rationale": "<why this matters>"
    }
  ]
}

Extract as many meaningful, distinct requirements as the data supports. Aim for at least 5 FRs and 3 NFRs.`

// tmplMoSCoW is the Stage 2 prompt: prioritize requirements using MoSCoW.
const tmplMoSCoW = `You are a Senior Requirements Engineer applying the MoSCoW prioritization method.

REQUIREMENTS LIST:
{{.RequirementsJSON}}

PROJECT CONTEXT:
{{.ProjectContext}}

MoSCoW DEFINITIONS:
- MUST   have: Critical. Without this, the system fails its core purpose.
- SHOULD have: Important but not vital. High value.
- COULD  have: Nice to have. Implement if time allows.
- WON'T  have: Explicitly out of scope for this version.

INSTRUCTIONS:
- Assign every requirement exactly one MoSCoW category.
- Add a short justification (1-2 sentences).
- Preserve the original requirement ID and statement unchanged.

OUTPUT FORMAT — respond ONLY with valid JSON:
{
  "moscow_prioritization": [
    {
      "id": "<original ID>",
      "statement": "<original statement>",
      "moscow": "MUST | SHOULD | COULD | WON'T",
      "justification": "<why this priority>"
    }
  ],
  "summary": {
    "must_count": 0,
    "should_count": 0,
    "could_count": 0,
    "wont_count": 0
  }
}`

// tmplDFD is the Stage 3 prompt: generate Data Flow Diagram components.
const tmplDFD = `You are a Senior Software Architect preparing a Data Flow Diagram (DFD) decomposition.

REQUIREMENTS:
{{.RequirementsJSON}}

YOUR TASK:
Identify all four DFD Level-1 components:

1. EXTERNAL ENTITIES — people or systems OUTSIDE the application
2. PROCESSES — internal functions the system performs (numbered P1, P2 ...)
3. DATA FLOWS — named arrows (Source -> Destination: DataName)
4. DATA STORES — persistent storage locations (D1, D2 ...)

RULES:
- Derive components ONLY from what the requirements describe.
- Every entity must have at least one flow in and one flow out.
- Every process must have at least one input and one output.
- Every data store must be read by at least one process and written by at least one process.

OUTPUT FORMAT — respond ONLY with valid JSON:
{
  "external_entities": [
    {
      "id": "E1",
      "name": "<entity name>",
      "description": "<what this entity represents>",
      "sends": ["<data flow names>"],
      "receives": ["<data flow names>"]
    }
  ],
  "processes": [
    {
      "id": "P1",
      "name": "<process name>",
      "description": "<what this process does>",
      "inputs": ["<data flow names>"],
      "outputs": ["<data flow names>"]
    }
  ],
  "data_stores": [
    {
      "id": "D1",
      "name": "<store name>",
      "description": "<what data is stored>",
      "read_by": ["<process IDs>"],
      "written_by": ["<process IDs>"]
    }
  ],
  "data_flows": [
    {
      "name": "<flow name>",
      "from": "<entity/process/store ID>",
      "to": "<entity/process/store ID>",
      "description": "<what data this flow carries>"
    }
  ]
}`

// tmplCSPEC is the Stage 4 prompt: generate Control Specification tables.
const tmplCSPEC = `You are a Senior Requirements Engineer preparing CSPEC (Control Specification) artefacts.

DFD COMPONENTS:
{{.DFDJSON}}

YOUR TASK:
For each significant PROCESS in the DFD, produce:

A) ACTIVATION TABLE — what event or input activates each process
B) DECISION TABLE — for processes with branching logic

RULES:
- Cover ALL processes from the DFD.
- Use Y (yes), N (no), or - (irrelevant) for condition values.
- Use X to mark which actions fire under each rule.

OUTPUT FORMAT — respond ONLY with valid JSON:
{
  "activation_tables": [
    {
      "process_id": "P1",
      "process_name": "<name>",
      "activations": [
        {
          "condition": "<what triggers this process>",
          "trigger_type": "event | data-arrival | timer | user-action",
          "description": "<brief explanation>"
        }
      ]
    }
  ],
  "decision_tables": [
    {
      "process_id": "P1",
      "process_name": "<name>",
      "conditions": ["<condition 1>", "<condition 2>"],
      "actions": ["<action 1>", "<action 2>"],
      "rules": [
        {
          "rule_id": "R1",
          "condition_values": ["Y", "N"],
          "action_values": ["X", "-"]
        }
      ]
    }
  ]
}`

// tmplSRS is the Stage 5 prompt: format everything into an IEEE 830 SRS document.
const tmplSRS = `You are a technical writer producing a Software Requirements Specification (SRS) document following the IEEE 830 standard.

PROJECT NAME: {{.ProjectName}}
VERSION: {{.Version}}
DATE: {{.Date}}

INPUT ARTEFACTS:

FUNCTIONAL & NON-FUNCTIONAL REQUIREMENTS:
{{.RequirementsJSON}}

MOSCOW PRIORITIZATION:
{{.MoscowJSON}}

DFD COMPONENTS:
{{.DFDJSON}}

CSPEC:
{{.CSPECJSON}}

STRICT FORMATTING RULES:

1. Use Markdown headings: ## for sections, ### for subsections.
2. Every requirement MUST start with "The system shall ..."
3. Use MoSCoW labels: MUST, SHOULD, COULD, WON'T (all caps).
4. Include EVERY requirement. Do not summarise or drop any.
5. Copy DFD components verbatim into tables.
6. Produce the complete document with all sections.

REQUIRED SECTIONS:

## 1. Introduction
### 1.1 Purpose
### 1.2 Scope
### 1.3 Definitions, Acronyms, and Abbreviations
### 1.4 References
### 1.5 Overview

## 2. Overall Description
### 2.1 Product Perspective
### 2.2 Product Functions
### 2.3 User Characteristics
### 2.4 Constraints
### 2.5 Assumptions and Dependencies

## 3. Specific Requirements
### 3.1 Functional Requirements
### 3.2 Non-Functional Requirements

## 4. System Models
### 4.1 Data Flow Diagram
(External Entities table, Processes table, Data Stores table, Data Flows table)
### 4.2 CSPEC — Control Specification
(Activation Table for all processes, Decision Table per process)

## 5. Appendices
### 5.1 MoSCoW Summary Table
### 5.2 Traceability Matrix

Produce the COMPLETE document. Do not add any commentary after the last section.`
