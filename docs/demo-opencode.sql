-- A single throwaway opencode session, for the README recordings.
--
-- opencode keeps its history in SQLite rather than in files, and a session
-- cannot be authored by hand the way docs/demo-seed.sh writes the Claude Code
-- and Codex ones: creating one means running a real model turn. So this is a
-- dump of a disposable opencode install instead, restored by demo-seed.sh.
--
-- It carries opencode's own 37-table schema, so the provider's query runs
-- against the real shape rather than a hand-made approximation of it.
--
-- Checked before committing: the message text was already replaced with
-- [redacted:...] markers by opencode itself, the only directory in here is
-- /private/tmp/demo-projects/storefront, and the account, credential and
-- control_account tables are empty. No real path, session, token or login.
--
-- The timestamps are frozen at the moment the dump was taken; demo-seed.sh
-- rewrites them to be relative to now, or a recording made later would show the
-- session as months old.
PRAGMA foreign_keys=OFF;
BEGIN TRANSACTION;
CREATE TABLE `workspace` (
          `id` text PRIMARY KEY,
          `type` text NOT NULL,
          `name` text DEFAULT '' NOT NULL,
          `branch` text,
          `directory` text,
          `extra` text,
          `project_id` text NOT NULL,
          `time_used` integer NOT NULL,
          CONSTRAINT `fk_workspace_project_id_project_id_fk` FOREIGN KEY (`project_id`) REFERENCES `project`(`id`) ON DELETE CASCADE
        );
CREATE TABLE `data_migration` (
          `name` text PRIMARY KEY,
          `time_completed` integer NOT NULL
        );
CREATE TABLE `account_state` (
          `id` integer PRIMARY KEY,
          `active_account_id` text,
          `active_org_id` text,
          CONSTRAINT `fk_account_state_active_account_id_account_id_fk` FOREIGN KEY (`active_account_id`) REFERENCES `account`(`id`) ON DELETE SET NULL
        );
CREATE TABLE `account` (
          `id` text PRIMARY KEY,
          `email` text NOT NULL,
          `url` text NOT NULL,
          `access_token` text NOT NULL,
          `refresh_token` text NOT NULL,
          `token_expiry` integer,
          `time_created` integer NOT NULL,
          `time_updated` integer NOT NULL
        );
CREATE TABLE `control_account` (
          `email` text NOT NULL,
          `url` text NOT NULL,
          `access_token` text NOT NULL,
          `refresh_token` text NOT NULL,
          `token_expiry` integer,
          `active` integer NOT NULL,
          `time_created` integer NOT NULL,
          `time_updated` integer NOT NULL,
          CONSTRAINT `control_account_pk` PRIMARY KEY(`email`, `url`)
        );
CREATE TABLE `credential` (
          `id` text PRIMARY KEY,
          `integration_id` text,
          `label` text NOT NULL,
          `value` text NOT NULL,
          `connector_id` text,
          `method_id` text,
          `active` integer,
          `time_created` integer NOT NULL,
          `time_updated` integer NOT NULL
        );
CREATE TABLE `event_sequence` (
          `aggregate_id` text PRIMARY KEY,
          `seq` integer NOT NULL,
          `owner_id` text
        );
CREATE TABLE `event` (
          `id` text PRIMARY KEY,
          `aggregate_id` text NOT NULL,
          `seq` integer NOT NULL,
          `type` text NOT NULL,
          `data` text NOT NULL,
          CONSTRAINT `fk_event_aggregate_id_event_sequence_aggregate_id_fk` FOREIGN KEY (`aggregate_id`) REFERENCES `event_sequence`(`aggregate_id`) ON DELETE CASCADE
        );
CREATE TABLE `permission` (
          `id` text PRIMARY KEY,
          `project_id` text NOT NULL,
          `action` text NOT NULL,
          `resource` text NOT NULL,
          `time_created` integer NOT NULL,
          `time_updated` integer NOT NULL,
          CONSTRAINT `fk_permission_project_id_project_id_fk` FOREIGN KEY (`project_id`) REFERENCES `project`(`id`) ON DELETE CASCADE
        );
CREATE TABLE `project_directory` (
          `project_id` text NOT NULL,
          `directory` text NOT NULL,
          `type` text,
          `strategy` text,
          `time_created` integer NOT NULL,
          CONSTRAINT `project_directory_pk` PRIMARY KEY(`project_id`, `directory`),
          CONSTRAINT `fk_project_directory_project_id_project_id_fk` FOREIGN KEY (`project_id`) REFERENCES `project`(`id`) ON DELETE CASCADE
        );
CREATE TABLE `project` (
          `id` text PRIMARY KEY,
          `worktree` text NOT NULL,
          `vcs` text,
          `name` text,
          `icon_url` text,
          `icon_url_override` text,
          `icon_color` text,
          `time_created` integer NOT NULL,
          `time_updated` integer NOT NULL,
          `time_initialized` integer,
          `sandboxes` text NOT NULL,
          `commands` text
        );
INSERT INTO project VALUES('global','/',NULL,NULL,NULL,NULL,NULL,1785189103614,1785189103614,NULL,'[]',NULL);
CREATE TABLE `message` (
          `id` text PRIMARY KEY,
          `session_id` text NOT NULL,
          `time_created` integer NOT NULL,
          `time_updated` integer NOT NULL,
          `data` text NOT NULL,
          CONSTRAINT `fk_message_session_id_session_id_fk` FOREIGN KEY (`session_id`) REFERENCES `session`(`id`) ON DELETE CASCADE
        );
INSERT INTO message VALUES('msg_fa449cdbd001k0oH1I4bCuiyhf','ses_05bb63252ffepU44GCZjI1svbM',1785167728061,1785189103652,'{"role":"user","time":{"created":1785167728061},"summary":{"diffs":[]},"agent":"build","model":{"providerID":"anthropic","modelID":"claude-sonnet-5"}}');
INSERT INTO message VALUES('msg_fa449cdcc001XNq1bnGnli5P9B','ses_05bb63252ffepU44GCZjI1svbM',1785167728076,1785189103652,'{"role":"assistant","time":{"created":1785167728076,"completed":1785167731376},"parentID":"msg_fa449cdbd001k0oH1I4bCuiyhf","modelID":"claude-sonnet-5","providerID":"anthropic","mode":"build","agent":"build","path":{"cwd":"[redacted:cwd:msg_fa449cdcc001XNq1bnGnli5P9B]","root":"[redacted:root:msg_fa449cdcc001XNq1bnGnli5P9B]"},"cost":0.031284,"tokens":{"total":12403,"input":2,"output":37,"reasoning":0,"cache":{"read":0,"write":12364}},"finish":"stop"}');
INSERT INTO message VALUES('msg_fa44a1358001EbgsGEe4H3z4p2','ses_05bb63252ffepU44GCZjI1svbM',1785167745880,1785189103653,'{"role":"user","time":{"created":1785167745880},"summary":{"diffs":[]},"agent":"build","model":{"providerID":"anthropic","modelID":"claude-sonnet-5"}}');
INSERT INTO message VALUES('msg_fa44a1363001VcPl1DSFMUYW2g','ses_05bb63252ffepU44GCZjI1svbM',1785167745891,1785189103653,'{"role":"assistant","time":{"created":1785167745891,"completed":1785167748658},"parentID":"msg_fa44a1358001EbgsGEe4H3z4p2","modelID":"claude-sonnet-5","providerID":"anthropic","mode":"build","agent":"build","path":{"cwd":"[redacted:cwd:msg_fa44a1363001VcPl1DSFMUYW2g]","root":"[redacted:root:msg_fa44a1363001VcPl1DSFMUYW2g]"},"cost":0.0028668,"tokens":{"total":12447,"input":2,"output":25,"reasoning":0,"cache":{"read":12364,"write":56}},"finish":"stop"}');
INSERT INTO message VALUES('msg_fa44a221b00141pO8QWlOrzwdL','ses_05bb63252ffepU44GCZjI1svbM',1785167749660,1785189103653,'{"role":"user","time":{"created":1785167749660},"summary":{"diffs":[]},"agent":"build","model":{"providerID":"anthropic","modelID":"claude-sonnet-5"}}');
INSERT INTO message VALUES('msg_fa44a2223001Xz3j1Y9Q4v1LpF','ses_05bb63252ffepU44GCZjI1svbM',1785167749667,1785189103653,'{"role":"assistant","time":{"created":1785167749667,"completed":1785167751938},"parentID":"msg_fa44a221b00141pO8QWlOrzwdL","modelID":"claude-sonnet-5","providerID":"anthropic","mode":"build","agent":"build","path":{"cwd":"[redacted:cwd:msg_fa44a2223001Xz3j1Y9Q4v1LpF]","root":"[redacted:root:msg_fa44a2223001Xz3j1Y9Q4v1LpF]"},"cost":0.0026305,"tokens":{"total":12461,"input":2,"output":6,"reasoning":0,"cache":{"read":12420,"write":33}},"finish":"stop"}');
INSERT INTO message VALUES('msg_fa44c3e5d0018Nk4GTErU1phTq','ses_05bb63252ffepU44GCZjI1svbM',1785167887965,1785189103654,'{"role":"user","time":{"created":1785167887965},"summary":{"diffs":[]},"agent":"build","model":{"providerID":"anthropic","modelID":"claude-sonnet-5"}}');
INSERT INTO message VALUES('msg_fa44c3e71001f8HF8oRTABqE8o','ses_05bb63252ffepU44GCZjI1svbM',1785167887985,1785189103654,'{"role":"assistant","time":{"created":1785167887985,"completed":1785167890718},"parentID":"msg_fa44c3e5d0018Nk4GTErU1phTq","modelID":"claude-sonnet-5","providerID":"anthropic","mode":"build","agent":"build","path":{"cwd":"[redacted:cwd:msg_fa44c3e71001f8HF8oRTABqE8o]","root":"[redacted:root:msg_fa44c3e71001f8HF8oRTABqE8o]"},"cost":0.031244,"tokens":{"total":12480,"input":2,"output":6,"reasoning":0,"cache":{"read":0,"write":12472}},"finish":"stop"}');
INSERT INTO message VALUES('msg_fa44ca8cb001WcQEbBzHEj4e7z','ses_05bb63252ffepU44GCZjI1svbM',1785167915212,1785189103654,'{"role":"user","time":{"created":1785167915212},"summary":{"diffs":[]},"agent":"build","model":{"providerID":"anthropic","modelID":"claude-sonnet-5"}}');
INSERT INTO message VALUES('msg_fa44ca92e001cCIgqE7RgI3mvf','ses_05bb63252ffepU44GCZjI1svbM',1785167915310,1785189103654,'{"role":"assistant","time":{"created":1785167915310,"completed":1785167918370},"parentID":"msg_fa44ca8cb001WcQEbBzHEj4e7z","modelID":"claude-sonnet-5","providerID":"anthropic","mode":"build","agent":"build","path":{"cwd":"[redacted:cwd:msg_fa44ca92e001cCIgqE7RgI3mvf]","root":"[redacted:root:msg_fa44ca92e001cCIgqE7RgI3mvf]"},"cost":0.0026234,"tokens":{"total":12494,"input":2,"output":10,"reasoning":0,"cache":{"read":12472,"write":10}},"finish":"stop"}');
CREATE TABLE `part` (
          `id` text PRIMARY KEY,
          `message_id` text NOT NULL,
          `session_id` text NOT NULL,
          `time_created` integer NOT NULL,
          `time_updated` integer NOT NULL,
          `data` text NOT NULL,
          CONSTRAINT `fk_part_message_id_message_id_fk` FOREIGN KEY (`message_id`) REFERENCES `message`(`id`) ON DELETE CASCADE
        );
INSERT INTO part VALUES('prt_fa449cdbf001JLUQ94Wi7ziVft','msg_fa449cdbd001k0oH1I4bCuiyhf','ses_05bb63252ffepU44GCZjI1svbM',1785189103652,1785189103652,'{"type":"text","text":"[redacted:text:prt_fa449cdbf001JLUQ94Wi7ziVft]"}');
INSERT INTO part VALUES('prt_fa449d8b5001tOSdegda5N6PD1','msg_fa449cdcc001XNq1bnGnli5P9B','ses_05bb63252ffepU44GCZjI1svbM',1785189103652,1785189103652,'{"type":"step-start"}');
INSERT INTO part VALUES('prt_fa449d8b7001Ve5eZ7UVphCdWr','msg_fa449cdcc001XNq1bnGnli5P9B','ses_05bb63252ffepU44GCZjI1svbM',1785189103653,1785189103653,'{"type":"text","text":"[redacted:text:prt_fa449d8b7001Ve5eZ7UVphCdWr]","time":{"start":1785167730871,"end":1785167731322}}');
INSERT INTO part VALUES('prt_fa449daac001RrxCVFpOczyqXm','msg_fa449cdcc001XNq1bnGnli5P9B','ses_05bb63252ffepU44GCZjI1svbM',1785189103653,1785189103653,'{"type":"step-finish","reason":"stop","cost":0.031284,"tokens":{"total":12403,"input":2,"output":37,"reasoning":0,"cache":{"read":0,"write":12364}}}');
INSERT INTO part VALUES('prt_fa44a1359001jvhRxCVP0MiNPt','msg_fa44a1358001EbgsGEe4H3z4p2','ses_05bb63252ffepU44GCZjI1svbM',1785189103653,1785189103653,'{"type":"text","text":"[redacted:text:prt_fa44a1359001jvhRxCVP0MiNPt]"}');
INSERT INTO part VALUES('prt_fa44a1d89001Jnt5ICVU0oNi3g','msg_fa44a1363001VcPl1DSFMUYW2g','ses_05bb63252ffepU44GCZjI1svbM',1785189103653,1785189103653,'{"type":"step-start"}');
INSERT INTO part VALUES('prt_fa44a1d8c001y2eV74x1vrMWIG','msg_fa44a1363001VcPl1DSFMUYW2g','ses_05bb63252ffepU44GCZjI1svbM',1785189103653,1785189103653,'{"type":"text","text":"[redacted:text:prt_fa44a1d8c001y2eV74x1vrMWIG]","time":{"start":1785167748492,"end":1785167748509}}');
INSERT INTO part VALUES('prt_fa44a1e2c001WTZ2RId4G09s2y','msg_fa44a1363001VcPl1DSFMUYW2g','ses_05bb63252ffepU44GCZjI1svbM',1785189103653,1785189103653,'{"type":"step-finish","reason":"stop","cost":0.0028668,"tokens":{"total":12447,"input":2,"output":25,"reasoning":0,"cache":{"read":12364,"write":56}}}');
INSERT INTO part VALUES('prt_fa44a221c001zNAbXEk8HbOWAB','msg_fa44a221b00141pO8QWlOrzwdL','ses_05bb63252ffepU44GCZjI1svbM',1785189103653,1785189103653,'{"type":"text","text":"[redacted:text:prt_fa44a221c001zNAbXEk8HbOWAB]"}');
INSERT INTO part VALUES('prt_fa44a2ae0001LlqoW3QCHTxhWq','msg_fa44a2223001Xz3j1Y9Q4v1LpF','ses_05bb63252ffepU44GCZjI1svbM',1785189103653,1785189103653,'{"type":"step-start"}');
INSERT INTO part VALUES('prt_fa44a2ae3001XrrgPoM7YR698X','msg_fa44a2223001Xz3j1Y9Q4v1LpF','ses_05bb63252ffepU44GCZjI1svbM',1785189103653,1785189103653,'{"type":"text","text":"[redacted:text:prt_fa44a2ae3001XrrgPoM7YR698X]","time":{"start":1785167751907,"end":1785167751932}}');
INSERT INTO part VALUES('prt_fa44a2aff001XOH7ps2dQMyvZD','msg_fa44a2223001Xz3j1Y9Q4v1LpF','ses_05bb63252ffepU44GCZjI1svbM',1785189103654,1785189103654,'{"type":"step-finish","reason":"stop","cost":0.0026305,"tokens":{"total":12461,"input":2,"output":6,"reasoning":0,"cache":{"read":12420,"write":33}}}');
INSERT INTO part VALUES('prt_fa44c3e5f0019PFa7622AM0sCc','msg_fa44c3e5d0018Nk4GTErU1phTq','ses_05bb63252ffepU44GCZjI1svbM',1785189103654,1785189103654,'{"type":"text","text":"[redacted:text:prt_fa44c3e5f0019PFa7622AM0sCc]"}');
INSERT INTO part VALUES('prt_fa44c49090017QEBVuyfzcPw1n','msg_fa44c3e71001f8HF8oRTABqE8o','ses_05bb63252ffepU44GCZjI1svbM',1785189103654,1785189103654,'{"type":"step-start"}');
INSERT INTO part VALUES('prt_fa44c490d0017fSO2OKmj6Tlz1','msg_fa44c3e71001f8HF8oRTABqE8o','ses_05bb63252ffepU44GCZjI1svbM',1785189103654,1785189103654,'{"type":"text","text":"[redacted:text:prt_fa44c490d0017fSO2OKmj6Tlz1]","time":{"start":1785167890701,"end":1785167890707}}');
INSERT INTO part VALUES('prt_fa44c4918001MWSpKRnw8Pm2Ac','msg_fa44c3e71001f8HF8oRTABqE8o','ses_05bb63252ffepU44GCZjI1svbM',1785189103654,1785189103654,'{"type":"step-finish","reason":"stop","cost":0.031244,"tokens":{"total":12480,"input":2,"output":6,"reasoning":0,"cache":{"read":0,"write":12472}}}');
INSERT INTO part VALUES('prt_fa44ca8ce001JuHIFCQBmYAa7B','msg_fa44ca8cb001WcQEbBzHEj4e7z','ses_05bb63252ffepU44GCZjI1svbM',1785189103654,1785189103654,'{"type":"text","text":"[redacted:text:prt_fa44ca8ce001JuHIFCQBmYAa7B]"}');
INSERT INTO part VALUES('prt_fa44cb4ac001iqIBP71iX87Kdz','msg_fa44ca92e001cCIgqE7RgI3mvf','ses_05bb63252ffepU44GCZjI1svbM',1785189103654,1785189103654,'{"type":"step-start"}');
INSERT INTO part VALUES('prt_fa44cb4ae0015y2goz32zFYsF2','msg_fa44ca92e001cCIgqE7RgI3mvf','ses_05bb63252ffepU44GCZjI1svbM',1785189103654,1785189103654,'{"type":"text","text":"[redacted:text:prt_fa44cb4ae0015y2goz32zFYsF2]","time":{"start":1785167918254,"end":1785167918263}}');
INSERT INTO part VALUES('prt_fa44cb520001chJ52SGzuBb8jq','msg_fa44ca92e001cCIgqE7RgI3mvf','ses_05bb63252ffepU44GCZjI1svbM',1785189103655,1785189103655,'{"type":"step-finish","reason":"stop","cost":0.0026234,"tokens":{"total":12494,"input":2,"output":10,"reasoning":0,"cache":{"read":12472,"write":10}}}');
CREATE TABLE `session_context_epoch` (
          `session_id` text PRIMARY KEY,
          `baseline` text NOT NULL,
          `snapshot` text NOT NULL,
          `baseline_seq` integer NOT NULL,
          CONSTRAINT `fk_session_context_epoch_session_id_session_id_fk` FOREIGN KEY (`session_id`) REFERENCES `session`(`id`) ON DELETE CASCADE
        );
CREATE TABLE `session_input` (
          `id` text PRIMARY KEY,
          `session_id` text NOT NULL,
          `prompt` text NOT NULL,
          `delivery` text NOT NULL,
          `admitted_seq` integer NOT NULL,
          `promoted_seq` integer,
          `time_created` integer NOT NULL,
          CONSTRAINT `fk_session_input_session_id_session_id_fk` FOREIGN KEY (`session_id`) REFERENCES `session`(`id`) ON DELETE CASCADE
        );
CREATE TABLE `session_message` (
          `id` text PRIMARY KEY,
          `session_id` text NOT NULL,
          `type` text NOT NULL,
          `seq` integer NOT NULL,
          `time_created` integer NOT NULL,
          `time_updated` integer NOT NULL,
          `data` text NOT NULL,
          CONSTRAINT `fk_session_message_session_id_session_id_fk` FOREIGN KEY (`session_id`) REFERENCES `session`(`id`) ON DELETE CASCADE
        );
CREATE TABLE `session` (
          `id` text PRIMARY KEY,
          `project_id` text NOT NULL,
          `workspace_id` text,
          `parent_id` text,
          `slug` text NOT NULL,
          `directory` text NOT NULL,
          `path` text,
          `title` text NOT NULL,
          `version` text NOT NULL,
          `share_url` text,
          `summary_additions` integer,
          `summary_deletions` integer,
          `summary_files` integer,
          `summary_diffs` text,
          `metadata` text,
          `cost` real DEFAULT 0 NOT NULL,
          `tokens_input` integer DEFAULT 0 NOT NULL,
          `tokens_output` integer DEFAULT 0 NOT NULL,
          `tokens_reasoning` integer DEFAULT 0 NOT NULL,
          `tokens_cache_read` integer DEFAULT 0 NOT NULL,
          `tokens_cache_write` integer DEFAULT 0 NOT NULL,
          `revert` text,
          `permission` text,
          `agent` text,
          `model` text,
          `time_created` integer NOT NULL,
          `time_updated` integer NOT NULL,
          `time_compacting` integer,
          `time_archived` integer,
          CONSTRAINT `fk_session_project_id_project_id_fk` FOREIGN KEY (`project_id`) REFERENCES `project`(`id`) ON DELETE CASCADE
        );
INSERT INTO session VALUES('ses_05bb63252ffepU44GCZjI1svbM','global',NULL,NULL,'demo-session','/private/tmp/demo-projects/storefront','private/tmp/demo-projects/storefront','rework the pricing table component','1.18.5',NULL,0,0,0,NULL,NULL,0.07064870000000000872,10,84,0,37256,24935,NULL,NULL,'build','{"id":"claude-sonnet-5","providerID":"anthropic"}',1785167728045,1785167918373,NULL,NULL);
CREATE TABLE `todo` (
          `session_id` text NOT NULL,
          `content` text NOT NULL,
          `status` text NOT NULL,
          `priority` text NOT NULL,
          `position` integer NOT NULL,
          `time_created` integer NOT NULL,
          `time_updated` integer NOT NULL,
          CONSTRAINT `todo_pk` PRIMARY KEY(`session_id`, `position`),
          CONSTRAINT `fk_todo_session_id_session_id_fk` FOREIGN KEY (`session_id`) REFERENCES `session`(`id`) ON DELETE CASCADE
        );
CREATE TABLE `session_share` (
          `session_id` text PRIMARY KEY,
          `id` text NOT NULL,
          `secret` text NOT NULL,
          `url` text NOT NULL,
          `time_created` integer NOT NULL,
          `time_updated` integer NOT NULL,
          CONSTRAINT `fk_session_share_session_id_session_id_fk` FOREIGN KEY (`session_id`) REFERENCES `session`(`id`) ON DELETE CASCADE
        );
CREATE TABLE IF NOT EXISTS "migration" (id TEXT PRIMARY KEY, time_completed INTEGER NOT NULL);
INSERT INTO migration VALUES('20260127222353_familiar_lady_ursula',1785189103549);
INSERT INTO migration VALUES('20260211171708_add_project_commands',1785189103549);
INSERT INTO migration VALUES('20260213144116_wakeful_the_professor',1785189103549);
INSERT INTO migration VALUES('20260225215848_workspace',1785189103549);
INSERT INTO migration VALUES('20260227213759_add_session_workspace_id',1785189103549);
INSERT INTO migration VALUES('20260228203230_blue_harpoon',1785189103549);
INSERT INTO migration VALUES('20260303231226_add_workspace_fields',1785189103549);
INSERT INTO migration VALUES('20260309230000_move_org_to_state',1785189103549);
INSERT INTO migration VALUES('20260312043431_session_message_cursor',1785189103549);
INSERT INTO migration VALUES('20260323234822_events',1785189103549);
INSERT INTO migration VALUES('20260410174513_workspace-name',1785189103549);
INSERT INTO migration VALUES('20260413175956_chief_energizer',1785189103549);
INSERT INTO migration VALUES('20260423070820_add_icon_url_override',1785189103549);
INSERT INTO migration VALUES('20260427172553_slow_nightmare',1785189103549);
INSERT INTO migration VALUES('20260428004200_add_session_path',1785189103549);
INSERT INTO migration VALUES('20260501142318_next_venus',1785189103549);
INSERT INTO migration VALUES('20260504145000_add_sync_owner',1785189103549);
INSERT INTO migration VALUES('20260507164347_add_workspace_time',1785189103549);
INSERT INTO migration VALUES('20260510033149_session_usage',1785189103549);
INSERT INTO migration VALUES('20260511000411_data_migration_state',1785189103549);
INSERT INTO migration VALUES('20260511173437_session-metadata',1785189103549);
INSERT INTO migration VALUES('20260601010001_normalize_storage_paths',1785189103549);
INSERT INTO migration VALUES('20260601202201_amazing_prowler',1785189103549);
INSERT INTO migration VALUES('20260602002951_lowly_union_jack',1785189103549);
INSERT INTO migration VALUES('20260602182828_add_project_directories',1785189103549);
INSERT INTO migration VALUES('20260603001617_session_message_projection_indexes',1785189103549);
INSERT INTO migration VALUES('20260603040000_session_message_projection_order',1785189103549);
INSERT INTO migration VALUES('20260603141458_session_input_inbox',1785189103549);
INSERT INTO migration VALUES('20260603160727_jittery_ezekiel_stane',1785189103549);
INSERT INTO migration VALUES('20260604172448_event_sourced_session_input',1785189103550);
INSERT INTO migration VALUES('20260605003541_add_session_context_snapshot',1785189103550);
INSERT INTO migration VALUES('20260605042240_add_context_epoch_agent',1785189103550);
INSERT INTO migration VALUES('20260611035744_credential',1785189103550);
INSERT INTO migration VALUES('20260611192811_lush_chimera',1785189103550);
INSERT INTO migration VALUES('20260612174303_project_dir_strategy',1785189103550);
INSERT INTO migration VALUES('20260622142730_simplify_session_context_epoch',1785189103550);
INSERT INTO migration VALUES('20260622170816_reset_v2_session_state',1785189103550);
INSERT INTO migration VALUES('20260622202450_simplify_session_input',1785189103550);
CREATE UNIQUE INDEX `event_aggregate_seq_idx` ON `event` (`aggregate_id`,`seq`);
CREATE INDEX `event_aggregate_type_seq_idx` ON `event` (`aggregate_id`,`type`,`seq`);
CREATE UNIQUE INDEX `permission_project_action_resource_idx` ON `permission` (`project_id`,`action`,`resource`);
CREATE INDEX `message_session_time_created_id_idx` ON `message` (`session_id`,`time_created`,`id`);
CREATE INDEX `part_message_id_id_idx` ON `part` (`message_id`,`id`);
CREATE INDEX `part_session_idx` ON `part` (`session_id`);
CREATE INDEX `session_input_session_pending_delivery_seq_idx` ON `session_input` (`session_id`,`promoted_seq`,`delivery`,`admitted_seq`);
CREATE UNIQUE INDEX `session_input_session_admitted_seq_idx` ON `session_input` (`session_id`,`admitted_seq`);
CREATE UNIQUE INDEX `session_input_session_promoted_seq_idx` ON `session_input` (`session_id`,`promoted_seq`);
CREATE UNIQUE INDEX `session_message_session_seq_idx` ON `session_message` (`session_id`,`seq`);
CREATE INDEX `session_message_session_type_seq_idx` ON `session_message` (`session_id`,`type`,`seq`);
CREATE INDEX `session_message_session_time_created_id_idx` ON `session_message` (`session_id`,`time_created`,`id`);
CREATE INDEX `session_message_time_created_idx` ON `session_message` (`time_created`);
CREATE INDEX `session_project_idx` ON `session` (`project_id`);
CREATE INDEX `session_workspace_idx` ON `session` (`workspace_id`);
CREATE INDEX `session_parent_idx` ON `session` (`parent_id`);
CREATE INDEX `todo_session_idx` ON `todo` (`session_id`);
COMMIT;
