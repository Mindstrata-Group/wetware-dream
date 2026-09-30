-- Full schema of a fresh Mindstrata database (generated from
-- apps/api/internal/testsupport/schema_base.sql). Postgres runs this file on
-- the first start of an empty data volume.

--
-- PostgreSQL database dump
--


-- Dumped from database version 17.10
-- Dumped by pg_dump version 17.10

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: SCHEMA public; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON SCHEMA public IS '';


--
-- Name: pg_stat_statements; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pg_stat_statements WITH SCHEMA public;


--
-- Name: EXTENSION pg_stat_statements; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION pg_stat_statements IS 'track planning and execution statistics of all SQL statements executed';


--
-- Name: prompt_type_enum; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.prompt_type_enum AS ENUM (
    'orchestrator',
    'summarization',
    'reminder'
);


--
-- Name: strip_mode_markdown(text); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.strip_mode_markdown(input text) RETURNS text
    LANGUAGE plpgsql IMMUTABLE
    AS $$
declare
  cleaned text;
begin
  if input is null then
    return null;
  end if;

  cleaned := input;

  -- Убираем только markdown-разметку, которая обычно попадает в промпты из редактора.
  cleaned := regexp_replace(cleaned, '(?m)^\s*```[[:alnum:]_-]*\s*$', '', 'g');
  cleaned := regexp_replace(cleaned, '\[([^\]\n]+)\]\([^)[:space:]]+(?:\s+"[^"]*")?\)', '\1', 'g');
  cleaned := regexp_replace(cleaned, '(\*\*|__)([^[:cntrl:]]*?)\1', '\2', 'g');
  cleaned := regexp_replace(cleaned, '(?m)^\s{0,3}#{1,6}[[:space:]]+', '', 'g');
  cleaned := regexp_replace(cleaned, '(?m)^\s{0,3}[-*+][[:space:]]+', '', 'g');
  cleaned := regexp_replace(cleaned, '(?m)^\s{0,3}[0-9]+[.)][[:space:]]+', '', 'g');
  cleaned := regexp_replace(cleaned, '[ \t]+\n', E'\n', 'g');
  cleaned := regexp_replace(cleaned, E'\n{3,}', E'\n\n', 'g');

  return btrim(cleaned, E' \t\r\n');
end;
$$;


--
-- Name: normalize_modes_markdown(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.normalize_modes_markdown() RETURNS TABLE(updated_rows bigint, saved_chars bigint)
    LANGUAGE plpgsql
    AS $$
begin
  return query
  with normalized as (
    select
      id,
      prompt as old_prompt,
      criteria as old_criteria,
      welcome_message as old_welcome_message,
      strip_mode_markdown(prompt) as new_prompt,
      strip_mode_markdown(criteria) as new_criteria,
      strip_mode_markdown(welcome_message) as new_welcome_message
    from modes
  ),
  changed as (
    update modes m
    set
      prompt = n.new_prompt,
      criteria = n.new_criteria,
      welcome_message = n.new_welcome_message
    from normalized n
    where m.id = n.id
      and (
        m.prompt is distinct from n.new_prompt
        or m.criteria is distinct from n.new_criteria
        or m.welcome_message is distinct from n.new_welcome_message
      )
    returning
      coalesce(length(n.old_prompt), 0)
        + coalesce(length(n.old_criteria), 0)
        + coalesce(length(n.old_welcome_message), 0)
        - coalesce(length(n.new_prompt), 0)
        - coalesce(length(n.new_criteria), 0)
        - coalesce(length(n.new_welcome_message), 0) as saved
  )
  select
    count(*)::bigint as updated_rows,
    coalesce(sum(greatest(saved, 0)), 0)::bigint as saved_chars
  from changed;
end;
$$;


--
-- Name: get_public_demo_modes_cached(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.get_public_demo_modes_cached() RETURNS TABLE(id bigint, name character varying, demo_chat text)
    LANGUAGE plpgsql
    AS $$
declare
	v_payload jsonb;
begin
	select c.payload
	into v_payload
	from public.public_demo_modes_cache c
	where c.cache_key = 'home_demo_modes'
	  and c.expires_at > now();

	if v_payload is null then
		perform pg_advisory_xact_lock(hashtext('home_demo_modes_cache_refresh')::bigint);

		select c.payload
		into v_payload
		from public.public_demo_modes_cache c
		where c.cache_key = 'home_demo_modes'
		  and c.expires_at > now();

		if v_payload is null then
			select coalesce(
				jsonb_agg(
					jsonb_build_object(
						'id', q.id,
						'name', q.name,
						'demo_chat', q.demo_chat
					)
					order by q.id
				),
				'[]'::jsonb
			)
			into v_payload
			from (
				select
					m.id,
					m.name,
					coalesce(m.demo_chat::text, '') as demo_chat
				from modes m
				where m.hidden_at is null
				  and m.demo_chat is not null
				  and m.demo_chat::text <> 'null'
				  and trim(m.demo_chat::text) <> ''
				  and exists (
					select 1
					from tariff_mode tm
					join tariffs t on t.id = tm.tariff_id
					left join tariff_groups tg on tg.id = t.group_id
					where tm.mode_id = m.id
					  and t.available_for_subscription = true
					  and t.tariff_type = 'regular'
					  and t.monthly_price > 0
					  and t.name not ilike 'AUTOPROMOCODETARIFF%'
					  and (
						tg.id is null
						or tg.available_for_subscription = true
					  )
				  )
				order by m.id asc
			) q;

			insert into public.public_demo_modes_cache (
				cache_key,
				payload,
				expires_at,
				updated_at
			)
			values (
				'home_demo_modes',
				v_payload,
				now() + interval '1 hour',
				now()
			)
			on conflict (cache_key) do update
			set payload = excluded.payload,
				expires_at = excluded.expires_at,
				updated_at = excluded.updated_at;
		end if;
	end if;

	return query
	select
		x.id,
		x.name,
		x.demo_chat
	from jsonb_to_recordset(v_payload) as x(
		id bigint,
		name varchar,
		demo_chat text
	)
	order by x.id asc;
end;
$$;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: admin_audit_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.admin_audit_log (
    id bigint NOT NULL,
    actor_user_id bigint,
    action text NOT NULL,
    target_type text NOT NULL,
    target_id bigint,
    ip_hash text,
    user_agent text,
    meta jsonb DEFAULT '{}'::jsonb NOT NULL,
    request_id text,
    outcome text DEFAULT 'success'::text NOT NULL,
    http_method text,
    path text,
    section text DEFAULT 'general'::text NOT NULL,
    sensitive boolean DEFAULT false NOT NULL,
    before_state jsonb DEFAULT '{}'::jsonb NOT NULL,
    after_state jsonb DEFAULT '{}'::jsonb NOT NULL,
    changed_fields text[] DEFAULT '{}'::text[] NOT NULL,
    error_code text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: push_subscriptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.push_subscriptions (
    id bigint GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    user_id bigint NOT NULL,
    endpoint text NOT NULL UNIQUE,
    p256dh text NOT NULL,
    auth text NOT NULL,
    user_agent text DEFAULT '' NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: notification_channel_queue; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.notification_channel_queue (
    id bigint GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    user_id bigint NOT NULL,
    channel text NOT NULL,
    text text NOT NULL,
    send_after timestamp with time zone NOT NULL,
    sent_at timestamp with time zone,
    failed text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: notification_consent_bonuses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.notification_consent_bonuses (
    user_id bigint NOT NULL,
    channel text NOT NULL,
    consent_type text NOT NULL,
    bonus_messages bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (user_id, channel, consent_type)
);


--
-- Name: admin_notification_sends; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.admin_notification_sends (
    id bigint GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    actor_id bigint NOT NULL,
    audience text NOT NULL,
    params jsonb DEFAULT '{}'::jsonb NOT NULL,
    channels text[] DEFAULT '{inbox}'::text[] NOT NULL,
    title text NOT NULL,
    body text NOT NULL,
    recipient_count integer DEFAULT 0 NOT NULL,
    delivered jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: admin_audit_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.admin_audit_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: admin_audit_log_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.admin_audit_log_id_seq OWNED BY public.admin_audit_log.id;


--
-- Name: admin_export_summaries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.admin_export_summaries (
    id bigint NOT NULL,
    actor_user_id bigint,
    promocode_id bigint,
    prompt_id bigint,
    prompt text NOT NULL,
    filters jsonb DEFAULT '{}'::jsonb NOT NULL,
    source_message_count integer DEFAULT 0 NOT NULL,
    source_bytes integer DEFAULT 0 NOT NULL,
    approx_tokens integer DEFAULT 0 NOT NULL,
    result text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: admin_export_summaries_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.admin_export_summaries_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: admin_export_summaries_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.admin_export_summaries_id_seq OWNED BY public.admin_export_summaries.id;


--
-- Name: admin_mode_usage_resets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.admin_mode_usage_resets (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    mode_id bigint NOT NULL,
    actor_user_id bigint,
    reset_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: admin_mode_usage_resets_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.admin_mode_usage_resets_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: admin_mode_usage_resets_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.admin_mode_usage_resets_id_seq OWNED BY public.admin_mode_usage_resets.id;


--
-- Name: admin_summary_prompts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.admin_summary_prompts (
    id bigint NOT NULL,
    name character varying NOT NULL,
    prompt text NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: admin_summary_prompts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.admin_summary_prompts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: admin_summary_prompts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.admin_summary_prompts_id_seq OWNED BY public.admin_summary_prompts.id;


--
-- Name: alembic_version; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.alembic_version (
    version_num character varying(32) NOT NULL
);


--
-- Name: argument_clinic_votes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.argument_clinic_votes (
    email_hash text NOT NULL,
    email text,
    vote text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT argument_clinic_votes_vote_check CHECK ((vote = ANY (ARRAY['virginia_ega'::text, 'amsterdam_network_psychometrics'::text, 'bridge'::text])))
);


--
-- Name: auth_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.auth_sessions (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    token_hash text NOT NULL,
    user_agent text,
    ip_hash text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone
);


--
-- Name: auth_sessions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.auth_sessions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: auth_sessions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.auth_sessions_id_seq OWNED BY public.auth_sessions.id;


--
-- Name: autopay_subscriptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.autopay_subscriptions (
    id bigint NOT NULL,
    subscription_id bigint NOT NULL,
    payment_method_id bigint NOT NULL,
    is_enabled boolean DEFAULT true NOT NULL,
    renewal_period_months integer NOT NULL,
    max_retry_attempts integer NOT NULL,
    retry_interval_hours integer NOT NULL,
    consecutive_failures integer NOT NULL,
    last_attempt_at timestamp with time zone,
    last_success_at timestamp with time zone,
    grace_until timestamp with time zone,
    next_retry_at timestamp with time zone,
    last_error text,
    cancel_reason character varying,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT max_retry_attempts_positive CHECK ((max_retry_attempts >= 0)),
    CONSTRAINT renewal_period_valid CHECK ((renewal_period_months = ANY (ARRAY[1, 3, 12])))
);


--
-- Name: autopay_subscriptions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.autopay_subscriptions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: autopay_subscriptions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.autopay_subscriptions_id_seq OWNED BY public.autopay_subscriptions.id;


--
-- Name: daily_message_counts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.daily_message_counts (
    user_id bigint NOT NULL,
    date date NOT NULL,
    count integer DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: daily_mode_usage; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.daily_mode_usage (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    mode_id bigint NOT NULL,
    access_id bigint NOT NULL,
    usage_date date NOT NULL,
    messages_used integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    text_messages_used integer,
    audio_messages_used integer
);


--
-- Name: daily_mode_usage_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.daily_mode_usage_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: daily_mode_usage_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.daily_mode_usage_id_seq OWNED BY public.daily_mode_usage.id;


--
-- Name: dialogs_messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.dialogs_messages (
    id bigint NOT NULL,
    dialog_id bigint NOT NULL,
    role character varying NOT NULL,
    content text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    compressed boolean DEFAULT false NOT NULL,
    is_compressed_context boolean DEFAULT false NOT NULL,
    anonymized_at timestamp with time zone
);


--
-- Name: dialogs_messages_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.dialogs_messages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: dialogs_messages_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.dialogs_messages_id_seq OWNED BY public.dialogs_messages.id;


--
-- Name: dialog_message_access_usage; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.dialog_message_access_usage (
    id bigint GENERATED BY DEFAULT AS IDENTITY NOT NULL,
    user_id bigint NOT NULL,
    mode_id bigint NOT NULL,
    dialog_message_id bigint NOT NULL,
    access_id bigint NOT NULL,
    usage_date date DEFAULT CURRENT_DATE NOT NULL,
    usage_kind character varying DEFAULT 'message'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT dialog_message_access_usage_kind_valid CHECK (((usage_kind)::text = ANY (ARRAY[('message'::character varying)::text, ('summary'::character varying)::text])))
);


--
-- Name: orchestration_decision_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.orchestration_decision_logs (
    id bigint GENERATED BY DEFAULT AS IDENTITY NOT NULL,
    user_id bigint NOT NULL,
    dialog_id bigint NOT NULL,
    current_mode_id bigint NOT NULL,
    selected_mode_id bigint,
    applied_mode_id bigint,
    model text DEFAULT ''::text NOT NULL,
    temperature double precision DEFAULT 0 NOT NULL,
    user_messages_since_switch integer DEFAULT 0 NOT NULL,
    check_interval integer DEFAULT 0 NOT NULL,
    candidate_mode_ids bigint[] DEFAULT '{}'::bigint[] NOT NULL,
    candidate_count integer DEFAULT 0 NOT NULL,
    decision text NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    raw_answer text DEFAULT ''::text NOT NULL,
    retry_raw_answer text DEFAULT ''::text NOT NULL,
    input_system text DEFAULT ''::text NOT NULL,
    input_user text DEFAULT ''::text NOT NULL,
    last_user_message text DEFAULT ''::text NOT NULL,
    response_format_used boolean DEFAULT false NOT NULL,
    retry_without_current boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: email_verification_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.email_verification_tokens (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    token_hash text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone
);


--
-- Name: email_verification_tokens_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.email_verification_tokens_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: email_verification_tokens_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.email_verification_tokens_id_seq OWNED BY public.email_verification_tokens.id;


--
-- Name: group_chats; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.group_chats (
    id bigint NOT NULL,
    chat_type character varying NOT NULL,
    title character varying,
    trigger_mode character varying DEFAULT 'mention_only'::character varying NOT NULL,
    trigger_keywords text[],
    max_context_messages integer DEFAULT 20 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone,
    message_counter integer DEFAULT 0 NOT NULL,
    trigger_every_n_messages integer DEFAULT 10 NOT NULL,
    user_id bigint NOT NULL,
    CONSTRAINT group_chat_type_valid CHECK (((chat_type)::text = ANY (ARRAY[('group'::character varying)::text, ('supergroup'::character varying)::text]))),
    CONSTRAINT group_trigger_mode_valid CHECK (((trigger_mode)::text = ANY (ARRAY[('mention_only'::character varying)::text, ('every_n_messages'::character varying)::text, ('keywords'::character varying)::text])))
);


--
-- Name: group_chats_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.group_chats_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: group_chats_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.group_chats_id_seq OWNED BY public.group_chats.id;


--
-- Name: invoices; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.invoices (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    tariff_id bigint NOT NULL,
    yookassa_payment_id character varying NOT NULL,
    amount numeric(10,2) NOT NULL,
    currency character varying NOT NULL,
    status character varying NOT NULL,
    subscription_months integer NOT NULL,
    is_recurring boolean DEFAULT false NOT NULL,
    autorenew_requested boolean DEFAULT false NOT NULL,
    yookassa_payment_method_id character varying,
    cancellation_reason character varying,
    renewal_attempt integer DEFAULT 0 NOT NULL,
    confirmation_url character varying,
    expires_at timestamp with time zone NOT NULL,
    paid_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: invoices_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.invoices_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: invoices_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.invoices_id_seq OWNED BY public.invoices.id;


--
-- Name: jwt_token_usage; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.jwt_token_usage (
    id bigint NOT NULL,
    token_hash character varying NOT NULL,
    summarization_count integer DEFAULT 0 NOT NULL,
    summarization_limit integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    last_used_at timestamp with time zone,
    CONSTRAINT check_summarization_count_non_negative CHECK ((summarization_count >= 0))
);


--
-- Name: jwt_token_usage_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.jwt_token_usage_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: jwt_token_usage_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.jwt_token_usage_id_seq OWNED BY public.jwt_token_usage.id;


--
-- Name: log_income; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.log_income (
    id bigint NOT NULL,
    amount numeric(10,2) NOT NULL,
    currency character varying NOT NULL,
    income_type character varying NOT NULL,
    subscription_id bigint,
    invoice_id bigint,
    promocode_id bigint,
    recurring_payment_attempt_id bigint,
    user_id bigint NOT NULL,
    payment_reference character varying,
    log_metadata text,
    description text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    paid boolean DEFAULT false NOT NULL,
    CONSTRAINT income_type_valid CHECK (((income_type)::text = ANY (ARRAY[('subscription'::character varying)::text, ('invoice_payment'::character varying)::text, ('recurring_subscription'::character varying)::text, ('paid_promocode'::character varying)::text])))
);


--
-- Name: log_income_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.log_income_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: log_income_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.log_income_id_seq OWNED BY public.log_income.id;


--
-- Name: message_terminologies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.message_terminologies (
    id bigint NOT NULL,
    name character varying NOT NULL,
    singular character varying NOT NULL,
    plural character varying NOT NULL,
    genitive character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: message_terminologies_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.message_terminologies_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: message_terminologies_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.message_terminologies_id_seq OWNED BY public.message_terminologies.id;


--
-- Name: message_usage; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.message_usage (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    mode_id bigint NOT NULL,
    dialog_message_id bigint NOT NULL,
    input_tokens integer NOT NULL,
    output_tokens integer NOT NULL,
    total_tokens integer NOT NULL,
    estimated_cost double precision,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    audio_seconds integer,
    access_id bigint,
    is_compressed_response boolean DEFAULT false NOT NULL
);


--
-- Name: message_usage_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.message_usage_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: message_usage_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.message_usage_id_seq OWNED BY public.message_usage.id;


--
-- Name: ai_provider_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_provider_events (
    id bigint NOT NULL,
    event_date date DEFAULT ((now() AT TIME ZONE 'utc'::text))::date NOT NULL,
    provider text NOT NULL,
    model text,
    outcome text NOT NULL,
    http_status integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ai_provider_events_outcome_check CHECK ((outcome = ANY (ARRAY['success'::text, 'attempt_failed'::text, 'exhausted'::text])))
);


--
-- Name: ai_provider_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.ai_provider_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ai_provider_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.ai_provider_events_id_seq OWNED BY public.ai_provider_events.id;


--
-- Name: mode_reminder_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mode_reminder_logs (
    id bigint NOT NULL,
    mode_usage_reminder_id bigint NOT NULL,
    user_id bigint NOT NULL,
    mode_id bigint NOT NULL,
    reminder_number integer NOT NULL,
    total_reminders integer NOT NULL,
    message_content text NOT NULL,
    generation_model character varying,
    prompt_id bigint,
    delivery_status character varying DEFAULT 'sent'::character varying NOT NULL,
    user_responded_at timestamp with time zone,
    sent_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT mode_reminder_delivery_status_valid CHECK (((delivery_status)::text = ANY (ARRAY[('sent'::character varying)::text, ('delivered'::character varying)::text, ('read'::character varying)::text, ('user_responded'::character varying)::text])))
);


--
-- Name: mode_reminder_logs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.mode_reminder_logs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: mode_reminder_logs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.mode_reminder_logs_id_seq OWNED BY public.mode_reminder_logs.id;


--
-- Name: mode_usage_reminders; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mode_usage_reminders (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    mode_id bigint NOT NULL,
    status character varying DEFAULT 'pending'::character varying NOT NULL,
    current_reminder_number integer DEFAULT 1 NOT NULL,
    total_reminders integer DEFAULT 3 NOT NULL,
    last_mode_usage_at timestamp with time zone NOT NULL,
    next_reminder_at timestamp with time zone NOT NULL,
    last_reminder_sent_at timestamp with time zone,
    workflow_id character varying,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT mode_reminder_status_valid CHECK (((status)::text = ANY (ARRAY[('pending'::character varying)::text, ('sent'::character varying)::text, ('completed'::character varying)::text, ('cancelled'::character varying)::text])))
);


--
-- Name: mode_usage_reminders_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.mode_usage_reminders_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: mode_usage_reminders_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.mode_usage_reminders_id_seq OWNED BY public.mode_usage_reminders.id;


--
-- Name: modes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.modes (
    id bigint NOT NULL,
    name character varying NOT NULL,
    ai_model character varying NOT NULL,
    model_temperature double precision NOT NULL,
    prompt text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    welcome_message text,
    terminology_id bigint,
    audio_enabled boolean DEFAULT false NOT NULL,
    audio_daily_limit integer,
    criteria text,
    orchestrator_check_interval integer DEFAULT 5 NOT NULL,
    reminder_count integer,
    reminder_interval_days integer DEFAULT 4 NOT NULL,
    reminder_tone text,
    reminder_prompt_id bigint,
    hidden_at timestamp with time zone,
    demo_chat jsonb,
    ai_provider text DEFAULT 'vsegpt'::text NOT NULL,
    thinking_mode text DEFAULT 'default'::text NOT NULL,
    ai_max_tokens integer,
    lead_notify_enabled boolean DEFAULT false NOT NULL,
    lead_notify_chat_ids text DEFAULT ''::text NOT NULL,
    lead_notify_threshold integer DEFAULT 3 NOT NULL,
    lead_notify_telegram_ids text DEFAULT ''::text NOT NULL
);


--
-- Name: modes_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.modes_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: modes_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.modes_id_seq OWNED BY public.modes.id;


--
-- Name: oauth_states; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.oauth_states (
    token_hash text NOT NULL,
    provider text NOT NULL,
    redirect_after text DEFAULT '/profile'::text NOT NULL,
    ip_hash text,
    user_agent text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone
);


--
-- Name: password_reset_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.password_reset_tokens (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    token_hash text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone
);


--
-- Name: password_reset_tokens_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.password_reset_tokens_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: password_reset_tokens_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.password_reset_tokens_id_seq OWNED BY public.password_reset_tokens.id;


--
-- Name: payment_methods; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.payment_methods (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    yookassa_payment_method_id character varying NOT NULL,
    payment_method_type character varying NOT NULL,
    status character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    last_used_at timestamp with time zone,
    CONSTRAINT payment_method_status_valid CHECK (((status)::text = ANY (ARRAY[('active'::character varying)::text, ('disabled'::character varying)::text, ('expired'::character varying)::text]))),
    CONSTRAINT payment_method_type_valid CHECK (((payment_method_type)::text = ANY (ARRAY[('bank_card'::character varying)::text, ('yoo_money'::character varying)::text])))
);


--
-- Name: payment_methods_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.payment_methods_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: payment_methods_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.payment_methods_id_seq OWNED BY public.payment_methods.id;


--
-- Name: promocode_targets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.promocode_targets (
    promocode_id bigint NOT NULL,
    target_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: promocode_usages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.promocode_usages (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    promocode_id bigint NOT NULL,
    used_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: promocode_usages_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.promocode_usages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: promocode_usages_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.promocode_usages_id_seq OWNED BY public.promocode_usages.id;


--
-- Name: promocodes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.promocodes (
    id bigint NOT NULL,
    code character varying NOT NULL,
    max_uses integer NOT NULL,
    used_count integer NOT NULL,
    active_from timestamp with time zone,
    active_to timestamp with time zone,
    duration interval NOT NULL,
    daily_message_limit integer DEFAULT 50 NOT NULL,
    access_priority integer NOT NULL,
    grants_type character varying NOT NULL,
    target_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    limit_type character varying NOT NULL,
    price numeric(10,2),
    comment text,
    purpose text,
    summary_limit integer,
    temporary_admin_enabled boolean DEFAULT false NOT NULL,
    temporary_admin_note text,
    first_mode_id bigint,
    CONSTRAINT grants_type_valid CHECK (((grants_type)::text = ANY (ARRAY[('mode'::character varying)::text, ('tariff'::character varying)::text, ('admin_role'::character varying)::text])))
);


--
-- Name: promocodes_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.promocodes_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: promocodes_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.promocodes_id_seq OWNED BY public.promocodes.id;


--
-- Name: prompts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.prompts (
    id bigint NOT NULL,
    name character varying NOT NULL,
    prompt_type public.prompt_type_enum NOT NULL,
    ai_model character varying NOT NULL,
    model_temperature double precision NOT NULL,
    system_prompt text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: prompts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.prompts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: prompts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.prompts_id_seq OWNED BY public.prompts.id;


--
-- Name: public_demo_modes_cache; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.public_demo_modes_cache (
    cache_key text NOT NULL,
    payload jsonb NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: billing_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.billing_runs (
    id bigserial PRIMARY KEY,
    run_key character varying NOT NULL UNIQUE,
    run_type character varying DEFAULT 'yookassa_daily'::character varying NOT NULL,
    source character varying NOT NULL,
    dry_run boolean DEFAULT false NOT NULL,
    status character varying DEFAULT 'running'::character varying NOT NULL,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    finished_at timestamp with time zone,
    totals jsonb DEFAULT '{}'::jsonb NOT NULL,
    error text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT billing_runs_source_valid CHECK (((source)::text = ANY (ARRAY[('temporal'::character varying)::text, ('admin'::character varying)::text]))),
    CONSTRAINT billing_runs_status_valid CHECK (((status)::text = ANY (ARRAY[('running'::character varying)::text, ('succeeded'::character varying)::text, ('failed'::character varying)::text])))
);


--
-- Name: billing_run_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.billing_run_items (
    id bigserial PRIMARY KEY,
    billing_run_id bigint NOT NULL REFERENCES public.billing_runs(id) ON DELETE CASCADE,
    step character varying NOT NULL,
    status character varying NOT NULL,
    counts jsonb DEFAULT '{}'::jsonb NOT NULL,
    error text,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    finished_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT billing_run_items_status_valid CHECK (((status)::text = ANY (ARRAY[('running'::character varying)::text, ('succeeded'::character varying)::text, ('failed'::character varying)::text, ('skipped'::character varying)::text]))),
    CONSTRAINT billing_run_items_step_unique UNIQUE (billing_run_id, step)
);


CREATE INDEX billing_runs_started_at_idx ON public.billing_runs USING btree (started_at DESC);


CREATE INDEX billing_run_items_billing_run_id_idx ON public.billing_run_items USING btree (billing_run_id);


--
-- Name: recurring_payment_attempts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.recurring_payment_attempts (
    id bigint NOT NULL,
    autopay_subscription_id bigint NOT NULL,
    yookassa_payment_id character varying NOT NULL,
    amount numeric(10,2) NOT NULL,
    currency character varying NOT NULL,
    attempt_number integer NOT NULL,
    status character varying NOT NULL,
    renewal_start_date timestamp with time zone NOT NULL,
    renewal_end_date timestamp with time zone NOT NULL,
    error_code character varying,
    error_description text,
    attempted_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    extra_data text,
    original_renewal_start_date timestamp with time zone NOT NULL,
    CONSTRAINT attempt_number_positive CHECK ((attempt_number > 0)),
    CONSTRAINT payment_attempt_status_valid CHECK (((status)::text = ANY (ARRAY[('pending'::character varying)::text, ('succeeded'::character varying)::text, ('failed'::character varying)::text, ('canceled'::character varying)::text])))
);


--
-- Name: recurring_payment_attempts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.recurring_payment_attempts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: recurring_payment_attempts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.recurring_payment_attempts_id_seq OWNED BY public.recurring_payment_attempts.id;


--
-- Name: reminder_configs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reminder_configs (
    id bigint NOT NULL,
    scope_type character varying NOT NULL,
    scope_id bigint,
    enabled boolean DEFAULT true NOT NULL,
    reminder_count integer DEFAULT 3 NOT NULL,
    reminder_intervals_hours text NOT NULL,
    inactivity_threshold_hours integer DEFAULT 24 NOT NULL,
    use_ai_generation boolean DEFAULT true NOT NULL,
    static_message_template text,
    reminder_prompt_id bigint,
    priority integer DEFAULT 100 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: reminder_configs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.reminder_configs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: reminder_configs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.reminder_configs_id_seq OWNED BY public.reminder_configs.id;


--
-- Name: reminder_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reminder_logs (
    id bigint NOT NULL,
    subscription_id bigint NOT NULL,
    reminder_offset_hours integer NOT NULL,
    sent_at timestamp with time zone DEFAULT now() NOT NULL,
    subscription_end timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: reminder_logs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.reminder_logs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: reminder_logs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.reminder_logs_id_seq OWNED BY public.reminder_logs.id;


--
-- Name: reminder_message_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reminder_message_logs (
    id bigint NOT NULL,
    user_inactivity_reminder_id bigint NOT NULL,
    user_id bigint NOT NULL,
    reminder_number integer NOT NULL,
    message_content text NOT NULL,
    was_ai_generated boolean DEFAULT false NOT NULL,
    delivery_status character varying DEFAULT 'sent'::character varying NOT NULL,
    sent_at timestamp with time zone DEFAULT now() NOT NULL,
    generation_model character varying,
    user_responded_at timestamp with time zone
);


--
-- Name: reminder_message_logs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.reminder_message_logs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: reminder_message_logs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.reminder_message_logs_id_seq OWNED BY public.reminder_message_logs.id;


--
-- Name: subscriptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.subscriptions (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    tariff_id bigint NOT NULL,
    status character varying NOT NULL,
    active_from timestamp with time zone NOT NULL,
    active_to timestamp with time zone NOT NULL,
    daily_message_limit integer,
    access_priority integer NOT NULL,
    payment_reference character varying,
    amount_paid numeric(10,2),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    autopay_renewal_months integer,
    autopay_disabled_reason character varying,
    is_recurring boolean DEFAULT false NOT NULL
);


--
-- Name: subscriptions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.subscriptions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: subscriptions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.subscriptions_id_seq OWNED BY public.subscriptions.id;


--
-- Name: system_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.system_settings (
    id bigint NOT NULL,
    key character varying NOT NULL,
    value text,
    description text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: system_settings_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.system_settings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: system_settings_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.system_settings_id_seq OWNED BY public.system_settings.id;


--
-- Name: tariff_groups; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tariff_groups (
    id bigint NOT NULL,
    name character varying NOT NULL,
    description text,
    terminology_id bigint,
    sort_order integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    available_for_subscription boolean DEFAULT false NOT NULL
);


--
-- Name: tariff_groups_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.tariff_groups_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: tariff_groups_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.tariff_groups_id_seq OWNED BY public.tariff_groups.id;


--
-- Name: tariff_mode; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tariff_mode (
    id bigint NOT NULL,
    tariff_id bigint NOT NULL,
    mode_id bigint NOT NULL,
    daily_message_limit integer DEFAULT 50 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: tariff_mode_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.tariff_mode_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: tariff_mode_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.tariff_mode_id_seq OWNED BY public.tariff_mode.id;


--
-- Name: tariffs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tariffs (
    id bigint NOT NULL,
    name character varying NOT NULL,
    description text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    monthly_price numeric(10,2) NOT NULL,
    yearly_price numeric(10,2),
    daily_message_limit integer DEFAULT 50 NOT NULL,
    limit_type character varying NOT NULL,
    group_id bigint,
    available_for_subscription boolean DEFAULT false NOT NULL,
    tariff_type character varying DEFAULT 'regular'::character varying NOT NULL,
    archived_at timestamp with time zone,
    first_mode_id bigint,
    CONSTRAINT check_limit_type_valid CHECK (((limit_type)::text = ANY (ARRAY[('shared'::character varying)::text, ('split'::character varying)::text]))),
    CONSTRAINT check_tariff_type_valid CHECK (((tariff_type)::text = ANY (ARRAY[('regular'::character varying)::text, ('promo'::character varying)::text])))
);


--
-- Name: tariffs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.tariffs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: tariffs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.tariffs_id_seq OWNED BY public.tariffs.id;


--
-- Name: user_identities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_identities (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    provider text NOT NULL,
    provider_user_id text NOT NULL,
    provider_email text,
    email_verified boolean DEFAULT false NOT NULL,
    display_name text,
    avatar_url text,
    raw_profile jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT user_identities_provider_check CHECK ((provider = 'yandex'::text))
);


--
-- Name: user_identities_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_identities_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_identities_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_identities_id_seq OWNED BY public.user_identities.id;


--
-- Name: notification_contacts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.notification_contacts (
    id bigserial PRIMARY KEY,
    user_id bigint NOT NULL,
    channel text NOT NULL,
    address text NOT NULL,
    verified boolean DEFAULT false NOT NULL,
    source text DEFAULT 'profile'::text NOT NULL,
    last_seen_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT notification_contacts_channel_check CHECK ((channel = ANY (ARRAY['email'::text, 'phone'::text, 'push'::text]))),
    CONSTRAINT notification_contacts_source_check CHECK ((source = ANY (ARRAY['profile'::text, 'yandex_id'::text, 'admin'::text, 'push_opt_in'::text]))),
    CONSTRAINT notification_contacts_user_channel_address_key UNIQUE (user_id, channel, address)
);


--
-- Name: notification_channel_consents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.notification_channel_consents (
    id bigserial PRIMARY KEY,
    user_id bigint NOT NULL,
    channel text NOT NULL,
    consent_type text NOT NULL,
    status text NOT NULL,
    source text DEFAULT 'profile'::text NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    granted_at timestamp with time zone,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT notification_channel_consents_channel_check CHECK ((channel = ANY (ARRAY['in_site'::text, 'email'::text, 'sms'::text, 'push'::text, 'messenger'::text]))),
    CONSTRAINT notification_channel_consents_type_check CHECK ((consent_type = ANY (ARRAY['service'::text, 'marketing'::text]))),
    CONSTRAINT notification_channel_consents_status_check CHECK ((status = ANY (ARRAY['granted'::text, 'denied'::text]))),
    CONSTRAINT notification_channel_consents_source_check CHECK ((source = ANY (ARRAY['profile'::text, 'admin'::text, 'scenario_opt_in'::text, 'migration'::text]))),
    CONSTRAINT notification_channel_consents_user_channel_type_key UNIQUE (user_id, channel, consent_type)
);


--
-- Name: notification_reachability; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.notification_reachability (
    user_id bigint PRIMARY KEY,
    email_available boolean DEFAULT false NOT NULL,
    phone_verified boolean DEFAULT false NOT NULL,
    push_enabled boolean DEFAULT false NOT NULL,
    service_channels jsonb DEFAULT '[]'::jsonb NOT NULL,
    marketing_channels jsonb DEFAULT '[]'::jsonb NOT NULL,
    best_channel text DEFAULT 'in_site'::text NOT NULL,
    last_interaction_channel text,
    last_interaction_at timestamp with time zone,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: notification_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.notification_templates (
    key text PRIMARY KEY,
    name text NOT NULL,
    consent_type text DEFAULT 'service'::text NOT NULL,
    default_channel text DEFAULT 'in_site'::text NOT NULL,
    title text NOT NULL,
    body text NOT NULL,
    active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT notification_templates_type_check CHECK ((consent_type = ANY (ARRAY['service'::text, 'marketing'::text]))),
    CONSTRAINT notification_templates_channel_check CHECK ((default_channel = ANY (ARRAY['in_site'::text, 'email'::text, 'sms'::text, 'push'::text, 'messenger'::text])))
);


--
-- Name: notification_inbox; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.notification_inbox (
    id bigserial PRIMARY KEY,
    user_id bigint NOT NULL,
    template_key text,
    consent_type text DEFAULT 'service'::text NOT NULL,
    title text NOT NULL,
    body text NOT NULL,
    status text DEFAULT 'unread'::text NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    read_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT notification_inbox_type_check CHECK ((consent_type = ANY (ARRAY['service'::text, 'marketing'::text]))),
    CONSTRAINT notification_inbox_status_check CHECK ((status = ANY (ARRAY['unread'::text, 'read'::text, 'archived'::text])))
);


--
-- Name: notification_delivery_attempts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.notification_delivery_attempts (
    id bigserial PRIMARY KEY,
    inbox_id bigint,
    user_id bigint NOT NULL,
    channel text NOT NULL,
    status text NOT NULL,
    provider text DEFAULT 'internal'::text NOT NULL,
    error text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT notification_delivery_attempts_channel_check CHECK ((channel = ANY (ARRAY['in_site'::text, 'email'::text, 'sms'::text, 'push'::text, 'messenger'::text]))),
    CONSTRAINT notification_delivery_attempts_status_check CHECK ((status = ANY (ARRAY['queued'::text, 'sent'::text, 'skipped_no_consent'::text, 'failed'::text])))
);


--
-- Name: user_inactivity_reminders; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_inactivity_reminders (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    dialog_id bigint NOT NULL,
    reminder_config_id bigint NOT NULL,
    status character varying DEFAULT 'pending'::character varying NOT NULL,
    current_reminder_number integer DEFAULT 1 NOT NULL,
    last_activity_at timestamp with time zone NOT NULL,
    next_reminder_at timestamp with time zone NOT NULL,
    last_reminder_sent_at timestamp with time zone,
    workflow_id character varying,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: user_inactivity_reminders_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_inactivity_reminders_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_inactivity_reminders_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_inactivity_reminders_id_seq OWNED BY public.user_inactivity_reminders.id;


--
-- Name: user_mode_access; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_mode_access (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    mode_id bigint NOT NULL,
    active_from timestamp with time zone NOT NULL,
    active_to timestamp with time zone NOT NULL,
    daily_message_limit integer DEFAULT 50 NOT NULL,
    priority integer NOT NULL,
    access_type character varying NOT NULL,
    source_id bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT access_type_valid CHECK (((access_type)::text = ANY (ARRAY[('promocode'::character varying)::text, ('subscription'::character varying)::text, ('manual'::character varying)::text])))
);


--
-- Name: user_mode_access_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_mode_access_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_mode_access_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_mode_access_id_seq OWNED BY public.user_mode_access.id;


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id bigint NOT NULL,
    telegram_id bigint,
    current_mode bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    current_dialog bigint,
    telegram_username character varying,
    orchestrator_enabled boolean DEFAULT false NOT NULL,
    accepted_tos boolean DEFAULT false NOT NULL,
    email text,
    password_hash text,
    role text DEFAULT 'user'::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    last_login_at timestamp with time zone,
    deleted_at timestamp with time zone,
    email_verified_at timestamp with time zone,
    display_name text,
    avatar_url text,
    phone text,
    allow_message_anonymization boolean DEFAULT true NOT NULL,
    max_chat_id bigint,
    max_bonus_granted boolean DEFAULT false NOT NULL,
    max_linked_at timestamp with time zone,
    telegram_linked_at timestamp with time zone,
    CONSTRAINT users_role_check CHECK ((role = ANY (ARRAY['user'::text, 'tester'::text, 'expert'::text, 'support'::text, 'content_admin'::text, 'billing_admin'::text, 'admin'::text, 'owner'::text]))),
    CONSTRAINT users_status_check CHECK ((status = ANY (ARRAY['active'::text, 'blocked'::text])))
);


--
-- Name: chat_file_blobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.chat_file_blobs (
    id bigint NOT NULL,
    sha256_hex character varying(64) NOT NULL,
    size_bytes bigint NOT NULL,
    source_bytes bytea NOT NULL,
    extracted_text text DEFAULT ''::text NOT NULL,
    prepared_text text DEFAULT ''::text NOT NULL,
    annotation_status character varying(16) DEFAULT 'direct'::character varying NOT NULL,
    annotation_model text DEFAULT ''::text NOT NULL,
    annotation_prompt text DEFAULT ''::text NOT NULL,
    annotated_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chat_file_blobs_annotation_status_check CHECK (((annotation_status)::text = ANY (ARRAY[('direct'::character varying)::text, ('annotated'::character varying)::text, ('failed'::character varying)::text])))
);


--
-- Name: chat_file_blobs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.chat_file_blobs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: chat_file_blobs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.chat_file_blobs_id_seq OWNED BY public.chat_file_blobs.id;


--
-- Name: chat_files; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.chat_files (
    id bigint NOT NULL,
    owner_user_id bigint NOT NULL,
    blob_id bigint NOT NULL,
    original_filename text NOT NULL,
    mime_type text DEFAULT ''::text NOT NULL,
    extension character varying(16) NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: chat_files_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.chat_files_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: chat_files_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.chat_files_id_seq OWNED BY public.chat_files.id;


--
-- Name: chat_message_attachments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.chat_message_attachments (
    id bigint NOT NULL,
    message_id bigint NOT NULL,
    file_id bigint NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: chat_message_attachments_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.chat_message_attachments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: chat_message_attachments_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.chat_message_attachments_id_seq OWNED BY public.chat_message_attachments.id;


--
-- Name: users_dialogs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users_dialogs (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    mode_id bigint NOT NULL,
    deleted_at timestamp with time zone,
    message_count bigint DEFAULT 0 NOT NULL,
    lead_notified_at timestamp with time zone
);


--
-- Name: users_dialogs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.users_dialogs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: users_dialogs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.users_dialogs_id_seq OWNED BY public.users_dialogs.id;


--
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: users_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.users_id_seq OWNED BY public.users.id;


--
-- Name: admin_audit_log id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_audit_log ALTER COLUMN id SET DEFAULT nextval('public.admin_audit_log_id_seq'::regclass);


--
-- Name: admin_export_summaries id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_export_summaries ALTER COLUMN id SET DEFAULT nextval('public.admin_export_summaries_id_seq'::regclass);


--
-- Name: admin_mode_usage_resets id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_mode_usage_resets ALTER COLUMN id SET DEFAULT nextval('public.admin_mode_usage_resets_id_seq'::regclass);


--
-- Name: admin_summary_prompts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_summary_prompts ALTER COLUMN id SET DEFAULT nextval('public.admin_summary_prompts_id_seq'::regclass);


--
-- Name: auth_sessions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_sessions ALTER COLUMN id SET DEFAULT nextval('public.auth_sessions_id_seq'::regclass);


--
-- Name: autopay_subscriptions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.autopay_subscriptions ALTER COLUMN id SET DEFAULT nextval('public.autopay_subscriptions_id_seq'::regclass);


--
-- Name: daily_mode_usage id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.daily_mode_usage ALTER COLUMN id SET DEFAULT nextval('public.daily_mode_usage_id_seq'::regclass);


--
-- Name: dialogs_messages id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dialogs_messages ALTER COLUMN id SET DEFAULT nextval('public.dialogs_messages_id_seq'::regclass);


--
-- Name: chat_file_blobs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_file_blobs ALTER COLUMN id SET DEFAULT nextval('public.chat_file_blobs_id_seq'::regclass);


--
-- Name: chat_files id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_files ALTER COLUMN id SET DEFAULT nextval('public.chat_files_id_seq'::regclass);


--
-- Name: chat_message_attachments id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_message_attachments ALTER COLUMN id SET DEFAULT nextval('public.chat_message_attachments_id_seq'::regclass);


--
-- Name: email_verification_tokens id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_verification_tokens ALTER COLUMN id SET DEFAULT nextval('public.email_verification_tokens_id_seq'::regclass);


--
-- Name: group_chats id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.group_chats ALTER COLUMN id SET DEFAULT nextval('public.group_chats_id_seq'::regclass);


--
-- Name: invoices id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoices ALTER COLUMN id SET DEFAULT nextval('public.invoices_id_seq'::regclass);


--
-- Name: jwt_token_usage id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.jwt_token_usage ALTER COLUMN id SET DEFAULT nextval('public.jwt_token_usage_id_seq'::regclass);


--
-- Name: log_income id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.log_income ALTER COLUMN id SET DEFAULT nextval('public.log_income_id_seq'::regclass);


--
-- Name: message_terminologies id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.message_terminologies ALTER COLUMN id SET DEFAULT nextval('public.message_terminologies_id_seq'::regclass);


--
-- Name: message_usage id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.message_usage ALTER COLUMN id SET DEFAULT nextval('public.message_usage_id_seq'::regclass);


--
-- Name: ai_provider_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_provider_events ALTER COLUMN id SET DEFAULT nextval('public.ai_provider_events_id_seq'::regclass);


--
-- Name: mode_reminder_logs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mode_reminder_logs ALTER COLUMN id SET DEFAULT nextval('public.mode_reminder_logs_id_seq'::regclass);


--
-- Name: mode_usage_reminders id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mode_usage_reminders ALTER COLUMN id SET DEFAULT nextval('public.mode_usage_reminders_id_seq'::regclass);


--
-- Name: modes id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.modes ALTER COLUMN id SET DEFAULT nextval('public.modes_id_seq'::regclass);


--
-- Name: password_reset_tokens id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.password_reset_tokens ALTER COLUMN id SET DEFAULT nextval('public.password_reset_tokens_id_seq'::regclass);


--
-- Name: payment_methods id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_methods ALTER COLUMN id SET DEFAULT nextval('public.payment_methods_id_seq'::regclass);


--
-- Name: promocode_usages id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.promocode_usages ALTER COLUMN id SET DEFAULT nextval('public.promocode_usages_id_seq'::regclass);


--
-- Name: promocodes id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.promocodes ALTER COLUMN id SET DEFAULT nextval('public.promocodes_id_seq'::regclass);


--
-- Name: prompts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompts ALTER COLUMN id SET DEFAULT nextval('public.prompts_id_seq'::regclass);


--
-- Name: recurring_payment_attempts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.recurring_payment_attempts ALTER COLUMN id SET DEFAULT nextval('public.recurring_payment_attempts_id_seq'::regclass);


--
-- Name: reminder_configs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reminder_configs ALTER COLUMN id SET DEFAULT nextval('public.reminder_configs_id_seq'::regclass);


--
-- Name: reminder_logs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reminder_logs ALTER COLUMN id SET DEFAULT nextval('public.reminder_logs_id_seq'::regclass);


--
-- Name: reminder_message_logs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reminder_message_logs ALTER COLUMN id SET DEFAULT nextval('public.reminder_message_logs_id_seq'::regclass);


--
-- Name: subscriptions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscriptions ALTER COLUMN id SET DEFAULT nextval('public.subscriptions_id_seq'::regclass);


--
-- Name: system_settings id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.system_settings ALTER COLUMN id SET DEFAULT nextval('public.system_settings_id_seq'::regclass);


--
-- Name: tariff_groups id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tariff_groups ALTER COLUMN id SET DEFAULT nextval('public.tariff_groups_id_seq'::regclass);


--
-- Name: tariff_mode id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tariff_mode ALTER COLUMN id SET DEFAULT nextval('public.tariff_mode_id_seq'::regclass);


--
-- Name: tariffs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tariffs ALTER COLUMN id SET DEFAULT nextval('public.tariffs_id_seq'::regclass);


--
-- Name: user_identities id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_identities ALTER COLUMN id SET DEFAULT nextval('public.user_identities_id_seq'::regclass);


--
-- Name: user_inactivity_reminders id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_inactivity_reminders ALTER COLUMN id SET DEFAULT nextval('public.user_inactivity_reminders_id_seq'::regclass);


--
-- Name: user_mode_access id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_mode_access ALTER COLUMN id SET DEFAULT nextval('public.user_mode_access_id_seq'::regclass);


--
-- Name: users id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users ALTER COLUMN id SET DEFAULT nextval('public.users_id_seq'::regclass);


--
-- Name: users_dialogs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users_dialogs ALTER COLUMN id SET DEFAULT nextval('public.users_dialogs_id_seq'::regclass);


--
-- Name: admin_audit_log admin_audit_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_audit_log
    ADD CONSTRAINT admin_audit_log_pkey PRIMARY KEY (id);


--
-- Name: admin_export_summaries admin_export_summaries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_export_summaries
    ADD CONSTRAINT admin_export_summaries_pkey PRIMARY KEY (id);


--
-- Name: admin_mode_usage_resets admin_mode_usage_resets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_mode_usage_resets
    ADD CONSTRAINT admin_mode_usage_resets_pkey PRIMARY KEY (id);


--
-- Name: admin_summary_prompts admin_summary_prompts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_summary_prompts
    ADD CONSTRAINT admin_summary_prompts_pkey PRIMARY KEY (id);


--
-- Name: alembic_version alembic_version_pkc; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alembic_version
    ADD CONSTRAINT alembic_version_pkc PRIMARY KEY (version_num);


--
-- Name: argument_clinic_votes argument_clinic_votes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.argument_clinic_votes
    ADD CONSTRAINT argument_clinic_votes_pkey PRIMARY KEY (email_hash);


--
-- Name: auth_sessions auth_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_sessions
    ADD CONSTRAINT auth_sessions_pkey PRIMARY KEY (id);


--
-- Name: auth_sessions auth_sessions_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_sessions
    ADD CONSTRAINT auth_sessions_token_hash_key UNIQUE (token_hash);


--
-- Name: autopay_subscriptions autopay_subscriptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.autopay_subscriptions
    ADD CONSTRAINT autopay_subscriptions_pkey PRIMARY KEY (id);


--
-- Name: autopay_subscriptions autopay_subscriptions_subscription_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.autopay_subscriptions
    ADD CONSTRAINT autopay_subscriptions_subscription_id_key UNIQUE (subscription_id);


--
-- Name: daily_message_counts daily_message_counts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.daily_message_counts
    ADD CONSTRAINT daily_message_counts_pkey PRIMARY KEY (user_id, date);


--
-- Name: daily_mode_usage daily_mode_usage_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.daily_mode_usage
    ADD CONSTRAINT daily_mode_usage_pkey PRIMARY KEY (id);


--
-- Name: dialogs_messages dialogs_messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dialogs_messages
    ADD CONSTRAINT dialogs_messages_pkey PRIMARY KEY (id);


--
-- Name: dialog_message_access_usage dialog_message_access_usage_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dialog_message_access_usage
    ADD CONSTRAINT dialog_message_access_usage_pkey PRIMARY KEY (id);


--
-- Name: dialog_message_access_usage uq_dialog_message_access_usage; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dialog_message_access_usage
    ADD CONSTRAINT uq_dialog_message_access_usage UNIQUE (dialog_message_id, access_id);


--
-- Name: orchestration_decision_logs orchestration_decision_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orchestration_decision_logs
    ADD CONSTRAINT orchestration_decision_logs_pkey PRIMARY KEY (id);


--
-- Name: chat_file_blobs chat_file_blobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_file_blobs
    ADD CONSTRAINT chat_file_blobs_pkey PRIMARY KEY (id);


--
-- Name: chat_files chat_files_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_files
    ADD CONSTRAINT chat_files_pkey PRIMARY KEY (id);


--
-- Name: chat_files chat_files_owner_blob_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_files
    ADD CONSTRAINT chat_files_owner_blob_unique UNIQUE (owner_user_id, blob_id);


--
-- Name: chat_message_attachments chat_message_attachments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_message_attachments
    ADD CONSTRAINT chat_message_attachments_pkey PRIMARY KEY (id);


--
-- Name: chat_message_attachments chat_message_attachments_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_message_attachments
    ADD CONSTRAINT chat_message_attachments_unique UNIQUE (message_id, file_id);


--
-- Name: email_verification_tokens email_verification_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_verification_tokens
    ADD CONSTRAINT email_verification_tokens_pkey PRIMARY KEY (id);


--
-- Name: email_verification_tokens email_verification_tokens_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_verification_tokens
    ADD CONSTRAINT email_verification_tokens_token_hash_key UNIQUE (token_hash);


--
-- Name: group_chats group_chats_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.group_chats
    ADD CONSTRAINT group_chats_pkey PRIMARY KEY (id);


--
-- Name: group_chats group_chats_user_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.group_chats
    ADD CONSTRAINT group_chats_user_id_key UNIQUE (user_id);


--
-- Name: invoices invoices_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoices
    ADD CONSTRAINT invoices_pkey PRIMARY KEY (id);


--
-- Name: invoices invoices_yookassa_payment_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoices
    ADD CONSTRAINT invoices_yookassa_payment_id_key UNIQUE (yookassa_payment_id);


--
-- Name: jwt_token_usage jwt_token_usage_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.jwt_token_usage
    ADD CONSTRAINT jwt_token_usage_pkey PRIMARY KEY (id);


--
-- Name: jwt_token_usage jwt_token_usage_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.jwt_token_usage
    ADD CONSTRAINT jwt_token_usage_token_hash_key UNIQUE (token_hash);


--
-- Name: log_income log_income_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.log_income
    ADD CONSTRAINT log_income_pkey PRIMARY KEY (id);


--
-- Name: message_terminologies message_terminologies_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.message_terminologies
    ADD CONSTRAINT message_terminologies_name_key UNIQUE (name);


--
-- Name: message_terminologies message_terminologies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.message_terminologies
    ADD CONSTRAINT message_terminologies_pkey PRIMARY KEY (id);


--
-- Name: message_usage message_usage_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.message_usage
    ADD CONSTRAINT message_usage_pkey PRIMARY KEY (id);


--
-- Name: ai_provider_events ai_provider_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_provider_events
    ADD CONSTRAINT ai_provider_events_pkey PRIMARY KEY (id);


--
-- Name: mode_reminder_logs mode_reminder_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mode_reminder_logs
    ADD CONSTRAINT mode_reminder_logs_pkey PRIMARY KEY (id);


--
-- Name: mode_usage_reminders mode_usage_reminders_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mode_usage_reminders
    ADD CONSTRAINT mode_usage_reminders_pkey PRIMARY KEY (id);


--
-- Name: modes modes_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.modes
    ADD CONSTRAINT modes_name_key UNIQUE (name);


--
-- Name: modes modes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.modes
    ADD CONSTRAINT modes_pkey PRIMARY KEY (id);


--
-- Name: oauth_states oauth_states_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.oauth_states
    ADD CONSTRAINT oauth_states_pkey PRIMARY KEY (token_hash);


--
-- Name: password_reset_tokens password_reset_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.password_reset_tokens
    ADD CONSTRAINT password_reset_tokens_pkey PRIMARY KEY (id);


--
-- Name: password_reset_tokens password_reset_tokens_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.password_reset_tokens
    ADD CONSTRAINT password_reset_tokens_token_hash_key UNIQUE (token_hash);


--
-- Name: payment_methods payment_methods_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_methods
    ADD CONSTRAINT payment_methods_pkey PRIMARY KEY (id);


--
-- Name: payment_methods payment_methods_yookassa_payment_method_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_methods
    ADD CONSTRAINT payment_methods_yookassa_payment_method_id_key UNIQUE (yookassa_payment_method_id);


--
-- Name: promocode_targets promocode_targets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.promocode_targets
    ADD CONSTRAINT promocode_targets_pkey PRIMARY KEY (promocode_id, target_id);


--
-- Name: promocode_usages promocode_usages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.promocode_usages
    ADD CONSTRAINT promocode_usages_pkey PRIMARY KEY (id);


--
-- Name: promocodes promocodes_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.promocodes
    ADD CONSTRAINT promocodes_code_key UNIQUE (code);


--
-- Name: promocodes promocodes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.promocodes
    ADD CONSTRAINT promocodes_pkey PRIMARY KEY (id);


--
-- Name: prompts prompts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompts
    ADD CONSTRAINT prompts_pkey PRIMARY KEY (id);


--
-- Name: public_demo_modes_cache public_demo_modes_cache_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.public_demo_modes_cache
    ADD CONSTRAINT public_demo_modes_cache_pkey PRIMARY KEY (cache_key);


--
-- Name: recurring_payment_attempts recurring_payment_attempts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.recurring_payment_attempts
    ADD CONSTRAINT recurring_payment_attempts_pkey PRIMARY KEY (id);


--
-- Name: recurring_payment_attempts recurring_payment_attempts_yookassa_payment_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.recurring_payment_attempts
    ADD CONSTRAINT recurring_payment_attempts_yookassa_payment_id_key UNIQUE (yookassa_payment_id);


--
-- Name: reminder_configs reminder_configs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reminder_configs
    ADD CONSTRAINT reminder_configs_pkey PRIMARY KEY (id);


--
-- Name: reminder_logs reminder_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reminder_logs
    ADD CONSTRAINT reminder_logs_pkey PRIMARY KEY (id);


--
-- Name: reminder_message_logs reminder_message_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reminder_message_logs
    ADD CONSTRAINT reminder_message_logs_pkey PRIMARY KEY (id);


--
-- Name: subscriptions subscriptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT subscriptions_pkey PRIMARY KEY (id);


--
-- Name: system_settings system_settings_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.system_settings
    ADD CONSTRAINT system_settings_key_key UNIQUE (key);


--
-- Name: system_settings system_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.system_settings
    ADD CONSTRAINT system_settings_pkey PRIMARY KEY (id);


--
-- Name: tariff_groups tariff_groups_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tariff_groups
    ADD CONSTRAINT tariff_groups_name_key UNIQUE (name);


--
-- Name: tariff_groups tariff_groups_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tariff_groups
    ADD CONSTRAINT tariff_groups_pkey PRIMARY KEY (id);


--
-- Name: tariff_mode tariff_mode_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tariff_mode
    ADD CONSTRAINT tariff_mode_pkey PRIMARY KEY (id);


--
-- Name: tariffs tariffs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tariffs
    ADD CONSTRAINT tariffs_pkey PRIMARY KEY (id);


--
-- Name: daily_mode_usage uq_daily_usage; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.daily_mode_usage
    ADD CONSTRAINT uq_daily_usage UNIQUE (user_id, mode_id, access_id, usage_date);


--
-- Name: promocode_usages uq_promocode_usage_user_promocode; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.promocode_usages
    ADD CONSTRAINT uq_promocode_usage_user_promocode UNIQUE (user_id, promocode_id);


--
-- Name: prompts uq_prompt_name_type; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompts
    ADD CONSTRAINT uq_prompt_name_type UNIQUE (name, prompt_type);


--
-- Name: reminder_configs uq_reminder_config_scope; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reminder_configs
    ADD CONSTRAINT uq_reminder_config_scope UNIQUE (scope_type, scope_id);


--
-- Name: reminder_logs uq_reminder_log_sub_offset_end; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reminder_logs
    ADD CONSTRAINT uq_reminder_log_sub_offset_end UNIQUE (subscription_id, reminder_offset_hours, subscription_end);


--
-- Name: user_mode_access uq_user_mode_access_source; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_mode_access
    ADD CONSTRAINT uq_user_mode_access_source UNIQUE (user_id, mode_id, access_type, source_id);


--
-- Name: user_identities user_identities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_identities
    ADD CONSTRAINT user_identities_pkey PRIMARY KEY (id);


--
-- Name: user_identities user_identities_provider_provider_user_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_identities
    ADD CONSTRAINT user_identities_provider_provider_user_id_key UNIQUE (provider, provider_user_id);


--
-- Name: user_inactivity_reminders user_inactivity_reminders_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_inactivity_reminders
    ADD CONSTRAINT user_inactivity_reminders_pkey PRIMARY KEY (id);


--
-- Name: user_mode_access user_mode_access_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_mode_access
    ADD CONSTRAINT user_mode_access_pkey PRIMARY KEY (id);


--
-- Name: users_dialogs users_dialogs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users_dialogs
    ADD CONSTRAINT users_dialogs_pkey PRIMARY KEY (id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: idx_users_telegram_id; Type: INDEX; Schema: public; Owner: -
-- (уникальность снята 20260702_150000: один TG-аккаунт — несколько аккаунтов сайта)
--

CREATE INDEX idx_users_telegram_id ON public.users USING btree (telegram_id) WHERE (telegram_id IS NOT NULL);


--
-- Name: admin_audit_log_actor_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX admin_audit_log_actor_idx ON public.admin_audit_log USING btree (actor_user_id, created_at DESC);

--
-- Name: admin_audit_log_request_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX admin_audit_log_request_id_idx ON public.admin_audit_log USING btree (request_id) WHERE (request_id IS NOT NULL);

--
-- Name: admin_audit_log_section_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX admin_audit_log_section_created_idx ON public.admin_audit_log USING btree (section, created_at DESC);

--
-- Name: admin_audit_log_sensitive_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX admin_audit_log_sensitive_created_idx ON public.admin_audit_log USING btree (created_at DESC) WHERE sensitive;


--
-- Name: idx_chat_file_blobs_sha_size; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_chat_file_blobs_sha_size ON public.chat_file_blobs USING btree (sha256_hex, size_bytes);


--
-- Name: idx_chat_message_attachments_file; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_chat_message_attachments_file ON public.chat_message_attachments USING btree (file_id);


--
-- Name: idx_chat_message_attachments_message; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_chat_message_attachments_message ON public.chat_message_attachments USING btree (message_id, "position", id);


--
-- Name: admin_audit_log_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX admin_audit_log_created_idx ON public.admin_audit_log USING btree (created_at DESC);


--
-- Name: admin_export_summaries_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX admin_export_summaries_created_idx ON public.admin_export_summaries USING btree (created_at DESC);


--
-- Name: admin_export_summaries_promocode_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX admin_export_summaries_promocode_idx ON public.admin_export_summaries USING btree (promocode_id, created_at DESC);


--
-- Name: admin_mode_usage_resets_user_mode_reset_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX admin_mode_usage_resets_user_mode_reset_idx ON public.admin_mode_usage_resets USING btree (user_id, mode_id, reset_at DESC);


--
-- Name: auth_sessions_user_active_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX auth_sessions_user_active_idx ON public.auth_sessions USING btree (user_id, expires_at DESC) WHERE (revoked_at IS NULL);


--
-- Name: email_verification_tokens_user_active_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX email_verification_tokens_user_active_idx ON public.email_verification_tokens USING btree (user_id, expires_at DESC) WHERE (used_at IS NULL);


--
-- Name: idx_daily_mode_usage_user_mode_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_daily_mode_usage_user_mode_date ON public.daily_mode_usage USING btree (user_id, mode_id, usage_date);


--
-- Name: idx_dialog_message_access_usage_access_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dialog_message_access_usage_access_date ON public.dialog_message_access_usage USING btree (access_id, usage_date);


--
-- Name: idx_dialog_message_access_usage_user_mode_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dialog_message_access_usage_user_mode_date ON public.dialog_message_access_usage USING btree (user_id, mode_id, usage_date);


--
-- Name: idx_orchestration_decision_logs_decision_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_orchestration_decision_logs_decision_created ON public.orchestration_decision_logs USING btree (decision, created_at DESC);


--
-- Name: idx_orchestration_decision_logs_dialog_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_orchestration_decision_logs_dialog_id ON public.orchestration_decision_logs USING btree (dialog_id, id DESC);


--
-- Name: idx_orchestration_decision_logs_user_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_orchestration_decision_logs_user_created ON public.orchestration_decision_logs USING btree (user_id, created_at DESC);


--
-- Name: idx_dialogs_messages_anon; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dialogs_messages_anon ON public.dialogs_messages USING btree (created_at, anonymized_at) WHERE (anonymized_at IS NULL);

-- Name: idx_dialogs_messages_created_at_role; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dialogs_messages_created_at_role ON public.dialogs_messages USING btree (created_at, role);


--
-- Name: idx_dialogs_messages_dialog_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dialogs_messages_dialog_created ON public.dialogs_messages USING btree (dialog_id, created_at);


--
-- Name: idx_dialogs_messages_dialog_created_id_desc; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dialogs_messages_dialog_created_id_desc ON public.dialogs_messages USING btree (dialog_id, created_at DESC, id DESC);


--
-- Name: idx_dialogs_messages_dialog_id_desc; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dialogs_messages_dialog_id_desc ON public.dialogs_messages USING btree (dialog_id, id DESC);


--
-- Name: idx_group_chats_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_group_chats_user_id ON public.group_chats USING btree (user_id);


--
-- Name: idx_jwt_token_usage_token_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_jwt_token_usage_token_hash ON public.jwt_token_usage USING btree (token_hash);


--
-- Name: idx_log_income_amount_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_log_income_amount_created ON public.log_income USING btree (amount, created_at);


--
-- Name: idx_log_income_type_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_log_income_type_created ON public.log_income USING btree (income_type, created_at);


--
-- Name: idx_log_income_user_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_log_income_user_created ON public.log_income USING btree (user_id, created_at);



--
-- Name: idx_message_usage_access_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_message_usage_access_id ON public.message_usage USING btree (access_id) WHERE (access_id IS NOT NULL);


--
-- Name: idx_message_usage_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_message_usage_created_at ON public.message_usage USING btree (created_at);


--
-- Name: idx_message_usage_mode_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_message_usage_mode_id ON public.message_usage USING btree (mode_id);


--
-- Name: idx_message_usage_user_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_message_usage_user_created ON public.message_usage USING btree (user_id, created_at);


--
-- Name: idx_ai_provider_events_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_provider_events_date ON public.ai_provider_events USING btree (event_date);


--
-- Name: idx_mode_reminder_logs_mode_sent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mode_reminder_logs_mode_sent ON public.mode_reminder_logs USING btree (mode_id, sent_at);


--
-- Name: idx_mode_reminder_logs_user_sent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mode_reminder_logs_user_sent ON public.mode_reminder_logs USING btree (user_id, sent_at);


--
-- Name: idx_mode_usage_reminders_next_reminder; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mode_usage_reminders_next_reminder ON public.mode_usage_reminders USING btree (next_reminder_at, status);


--
-- Name: idx_mode_usage_reminders_user_mode; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mode_usage_reminders_user_mode ON public.mode_usage_reminders USING btree (user_id, mode_id);


--
-- Name: idx_mode_usage_reminders_user_mode_active; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_mode_usage_reminders_user_mode_active ON public.mode_usage_reminders USING btree (user_id, mode_id) WHERE ((status)::text = ANY (ARRAY[('pending'::character varying)::text, ('sent'::character varying)::text]));


--
-- Name: idx_mode_usage_reminders_user_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mode_usage_reminders_user_status ON public.mode_usage_reminders USING btree (user_id, status);


--
-- Name: idx_mode_usage_reminders_workflow; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mode_usage_reminders_workflow ON public.mode_usage_reminders USING btree (workflow_id);


--
-- Name: idx_promocode_usages_promocode_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_promocode_usages_promocode_id ON public.promocode_usages USING btree (promocode_id);


--
-- Name: idx_reminder_configs_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reminder_configs_scope ON public.reminder_configs USING btree (scope_type, scope_id);


--
-- Name: idx_reminder_message_logs_user_sent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reminder_message_logs_user_sent ON public.reminder_message_logs USING btree (user_id, sent_at);


--
-- Name: idx_subscriptions_user_status_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscriptions_user_status_active ON public.subscriptions USING btree (user_id, status, active_to);


--
-- Name: idx_tariff_mode_mode_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tariff_mode_mode_id ON public.tariff_mode USING btree (mode_id);


--
-- Name: idx_tariff_mode_tariff_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tariff_mode_tariff_id ON public.tariff_mode USING btree (tariff_id);


--
-- Name: idx_user_inactivity_reminders_next_reminder; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_inactivity_reminders_next_reminder ON public.user_inactivity_reminders USING btree (next_reminder_at, status);


--
-- Name: idx_user_inactivity_reminders_user_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_inactivity_reminders_user_status ON public.user_inactivity_reminders USING btree (user_id, status);


--
-- Name: idx_user_inactivity_reminders_workflow; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_inactivity_reminders_workflow ON public.user_inactivity_reminders USING btree (workflow_id);


--
-- Name: idx_user_mode_access_user_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_mode_access_user_active ON public.user_mode_access USING btree (user_id, mode_id, active_from, active_to);


--
-- Name: idx_user_mode_access_user_active_period; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_mode_access_user_active_period ON public.user_mode_access USING btree (user_id, active_from, active_to);


--
-- Name: idx_users_dialogs_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_dialogs_user_id ON public.users_dialogs USING btree (user_id) WHERE (deleted_at IS NULL);


--
-- Name: ix_prompts_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_prompts_name ON public.prompts USING btree (name);


--
-- Name: ix_prompts_prompt_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_prompts_prompt_type ON public.prompts USING btree (prompt_type);


--
-- Name: oauth_states_active_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX oauth_states_active_idx ON public.oauth_states USING btree (provider, expires_at DESC) WHERE (used_at IS NULL);


--
-- Name: password_reset_tokens_user_active_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX password_reset_tokens_user_active_idx ON public.password_reset_tokens USING btree (user_id, expires_at DESC) WHERE (used_at IS NULL);


--
-- Name: promocode_targets_target_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX promocode_targets_target_idx ON public.promocode_targets USING btree (target_id);


--
-- Name: user_identities_user_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX user_identities_user_idx ON public.user_identities USING btree (user_id, provider);


--
-- Name: users_email_lower_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX users_email_lower_uq ON public.users USING btree (lower(email)) WHERE ((email IS NOT NULL) AND (deleted_at IS NULL));


--
-- Name: users_phone_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX users_phone_idx ON public.users USING btree (phone) WHERE ((phone IS NOT NULL) AND (deleted_at IS NULL));


--
-- Name: users_role_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX users_role_status_idx ON public.users USING btree (role, status) WHERE (deleted_at IS NULL);


--
-- Name: users_status_email_verified_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX users_status_email_verified_idx ON public.users USING btree (status, email_verified_at) WHERE (deleted_at IS NULL);


--
-- Name: idx_users_max_chat_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_max_chat_id ON public.users USING btree (max_chat_id) WHERE (max_chat_id IS NOT NULL);


--
-- Name: admin_audit_log admin_audit_log_actor_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_audit_log
    ADD CONSTRAINT admin_audit_log_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: admin_mode_usage_resets admin_mode_usage_resets_actor_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_mode_usage_resets
    ADD CONSTRAINT admin_mode_usage_resets_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: admin_mode_usage_resets admin_mode_usage_resets_mode_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_mode_usage_resets
    ADD CONSTRAINT admin_mode_usage_resets_mode_id_fkey FOREIGN KEY (mode_id) REFERENCES public.modes(id) ON DELETE CASCADE;


--
-- Name: admin_mode_usage_resets admin_mode_usage_resets_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_mode_usage_resets
    ADD CONSTRAINT admin_mode_usage_resets_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: auth_sessions auth_sessions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_sessions
    ADD CONSTRAINT auth_sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: autopay_subscriptions autopay_subscriptions_payment_method_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.autopay_subscriptions
    ADD CONSTRAINT autopay_subscriptions_payment_method_id_fkey FOREIGN KEY (payment_method_id) REFERENCES public.payment_methods(id);


--
-- Name: autopay_subscriptions autopay_subscriptions_subscription_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.autopay_subscriptions
    ADD CONSTRAINT autopay_subscriptions_subscription_id_fkey FOREIGN KEY (subscription_id) REFERENCES public.subscriptions(id);


--
-- Name: daily_mode_usage daily_mode_usage_access_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.daily_mode_usage
    ADD CONSTRAINT daily_mode_usage_access_id_fkey FOREIGN KEY (access_id) REFERENCES public.user_mode_access(id);


--
-- Name: daily_mode_usage daily_mode_usage_mode_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.daily_mode_usage
    ADD CONSTRAINT daily_mode_usage_mode_id_fkey FOREIGN KEY (mode_id) REFERENCES public.modes(id);


--
-- Name: daily_mode_usage daily_mode_usage_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.daily_mode_usage
    ADD CONSTRAINT daily_mode_usage_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: dialogs_messages dialogs_messages_dialog_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dialogs_messages
    ADD CONSTRAINT dialogs_messages_dialog_id_fkey FOREIGN KEY (dialog_id) REFERENCES public.users_dialogs(id);


--
-- Name: dialog_message_access_usage dialog_message_access_usage_access_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dialog_message_access_usage
    ADD CONSTRAINT dialog_message_access_usage_access_id_fkey FOREIGN KEY (access_id) REFERENCES public.user_mode_access(id);


--
-- Name: dialog_message_access_usage dialog_message_access_usage_dialog_message_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dialog_message_access_usage
    ADD CONSTRAINT dialog_message_access_usage_dialog_message_id_fkey FOREIGN KEY (dialog_message_id) REFERENCES public.dialogs_messages(id) ON DELETE CASCADE;


--
-- Name: dialog_message_access_usage dialog_message_access_usage_mode_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dialog_message_access_usage
    ADD CONSTRAINT dialog_message_access_usage_mode_id_fkey FOREIGN KEY (mode_id) REFERENCES public.modes(id);


--
-- Name: dialog_message_access_usage dialog_message_access_usage_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dialog_message_access_usage
    ADD CONSTRAINT dialog_message_access_usage_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: orchestration_decision_logs orchestration_decision_logs_applied_mode_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orchestration_decision_logs
    ADD CONSTRAINT orchestration_decision_logs_applied_mode_id_fkey FOREIGN KEY (applied_mode_id) REFERENCES public.modes(id);


--
-- Name: orchestration_decision_logs orchestration_decision_logs_current_mode_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orchestration_decision_logs
    ADD CONSTRAINT orchestration_decision_logs_current_mode_id_fkey FOREIGN KEY (current_mode_id) REFERENCES public.modes(id);


--
-- Name: orchestration_decision_logs orchestration_decision_logs_dialog_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orchestration_decision_logs
    ADD CONSTRAINT orchestration_decision_logs_dialog_id_fkey FOREIGN KEY (dialog_id) REFERENCES public.users_dialogs(id) ON DELETE CASCADE;


--
-- Name: orchestration_decision_logs orchestration_decision_logs_selected_mode_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orchestration_decision_logs
    ADD CONSTRAINT orchestration_decision_logs_selected_mode_id_fkey FOREIGN KEY (selected_mode_id) REFERENCES public.modes(id);


--
-- Name: orchestration_decision_logs orchestration_decision_logs_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orchestration_decision_logs
    ADD CONSTRAINT orchestration_decision_logs_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: chat_files chat_files_owner_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_files
    ADD CONSTRAINT chat_files_owner_user_id_fkey FOREIGN KEY (owner_user_id) REFERENCES public.users(id);


--
-- Name: chat_files chat_files_blob_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_files
    ADD CONSTRAINT chat_files_blob_id_fkey FOREIGN KEY (blob_id) REFERENCES public.chat_file_blobs(id);


--
-- Name: chat_message_attachments chat_message_attachments_file_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_message_attachments
    ADD CONSTRAINT chat_message_attachments_file_id_fkey FOREIGN KEY (file_id) REFERENCES public.chat_files(id);


--
-- Name: chat_message_attachments chat_message_attachments_message_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_message_attachments
    ADD CONSTRAINT chat_message_attachments_message_id_fkey FOREIGN KEY (message_id) REFERENCES public.dialogs_messages(id) ON DELETE CASCADE;


--
-- Name: email_verification_tokens email_verification_tokens_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_verification_tokens
    ADD CONSTRAINT email_verification_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: group_chats group_chats_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.group_chats
    ADD CONSTRAINT group_chats_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: invoices invoices_tariff_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoices
    ADD CONSTRAINT invoices_tariff_id_fkey FOREIGN KEY (tariff_id) REFERENCES public.tariffs(id);


--
-- Name: invoices invoices_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoices
    ADD CONSTRAINT invoices_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: log_income log_income_invoice_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.log_income
    ADD CONSTRAINT log_income_invoice_id_fkey FOREIGN KEY (invoice_id) REFERENCES public.invoices(id);


--
-- Name: log_income log_income_promocode_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.log_income
    ADD CONSTRAINT log_income_promocode_id_fkey FOREIGN KEY (promocode_id) REFERENCES public.promocodes(id);


--
-- Name: log_income log_income_recurring_payment_attempt_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.log_income
    ADD CONSTRAINT log_income_recurring_payment_attempt_id_fkey FOREIGN KEY (recurring_payment_attempt_id) REFERENCES public.recurring_payment_attempts(id);


--
-- Name: log_income log_income_subscription_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.log_income
    ADD CONSTRAINT log_income_subscription_id_fkey FOREIGN KEY (subscription_id) REFERENCES public.subscriptions(id);


--
-- Name: log_income log_income_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.log_income
    ADD CONSTRAINT log_income_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: message_usage message_usage_access_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.message_usage
    ADD CONSTRAINT message_usage_access_id_fkey FOREIGN KEY (access_id) REFERENCES public.user_mode_access(id);


--
-- Name: message_usage message_usage_dialog_message_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.message_usage
    ADD CONSTRAINT message_usage_dialog_message_id_fkey FOREIGN KEY (dialog_message_id) REFERENCES public.dialogs_messages(id);


--
-- Name: message_usage message_usage_mode_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.message_usage
    ADD CONSTRAINT message_usage_mode_id_fkey FOREIGN KEY (mode_id) REFERENCES public.modes(id);


--
-- Name: message_usage message_usage_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.message_usage
    ADD CONSTRAINT message_usage_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: mode_reminder_logs mode_reminder_logs_mode_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mode_reminder_logs
    ADD CONSTRAINT mode_reminder_logs_mode_id_fkey FOREIGN KEY (mode_id) REFERENCES public.modes(id) ON DELETE CASCADE;


--
-- Name: mode_reminder_logs mode_reminder_logs_mode_usage_reminder_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mode_reminder_logs
    ADD CONSTRAINT mode_reminder_logs_mode_usage_reminder_id_fkey FOREIGN KEY (mode_usage_reminder_id) REFERENCES public.mode_usage_reminders(id) ON DELETE CASCADE;


--
-- Name: mode_reminder_logs mode_reminder_logs_prompt_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mode_reminder_logs
    ADD CONSTRAINT mode_reminder_logs_prompt_id_fkey FOREIGN KEY (prompt_id) REFERENCES public.prompts(id) ON DELETE SET NULL;


--
-- Name: mode_reminder_logs mode_reminder_logs_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mode_reminder_logs
    ADD CONSTRAINT mode_reminder_logs_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: mode_usage_reminders mode_usage_reminders_mode_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mode_usage_reminders
    ADD CONSTRAINT mode_usage_reminders_mode_id_fkey FOREIGN KEY (mode_id) REFERENCES public.modes(id) ON DELETE CASCADE;


--
-- Name: mode_usage_reminders mode_usage_reminders_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mode_usage_reminders
    ADD CONSTRAINT mode_usage_reminders_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: modes modes_reminder_prompt_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.modes
    ADD CONSTRAINT modes_reminder_prompt_id_fkey FOREIGN KEY (reminder_prompt_id) REFERENCES public.prompts(id) ON DELETE SET NULL;


--
-- Name: modes modes_terminology_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.modes
    ADD CONSTRAINT modes_terminology_id_fkey FOREIGN KEY (terminology_id) REFERENCES public.message_terminologies(id);


--
-- Name: password_reset_tokens password_reset_tokens_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.password_reset_tokens
    ADD CONSTRAINT password_reset_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: payment_methods payment_methods_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_methods
    ADD CONSTRAINT payment_methods_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: promocode_targets promocode_targets_promocode_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.promocode_targets
    ADD CONSTRAINT promocode_targets_promocode_id_fkey FOREIGN KEY (promocode_id) REFERENCES public.promocodes(id) ON DELETE CASCADE;


--
-- Name: promocode_usages promocode_usages_promocode_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.promocode_usages
    ADD CONSTRAINT promocode_usages_promocode_id_fkey FOREIGN KEY (promocode_id) REFERENCES public.promocodes(id);


-- first_mode_id: mode_id (grants_type=mode) или tariff_id (grants_type=tariff), FK не ставим.


--
-- Name: promocode_usages promocode_usages_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.promocode_usages
    ADD CONSTRAINT promocode_usages_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: recurring_payment_attempts recurring_payment_attempts_autopay_subscription_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.recurring_payment_attempts
    ADD CONSTRAINT recurring_payment_attempts_autopay_subscription_id_fkey FOREIGN KEY (autopay_subscription_id) REFERENCES public.autopay_subscriptions(id);


--
-- Name: reminder_configs reminder_configs_reminder_prompt_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reminder_configs
    ADD CONSTRAINT reminder_configs_reminder_prompt_id_fkey FOREIGN KEY (reminder_prompt_id) REFERENCES public.prompts(id);


--
-- Name: reminder_logs reminder_logs_subscription_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reminder_logs
    ADD CONSTRAINT reminder_logs_subscription_id_fkey FOREIGN KEY (subscription_id) REFERENCES public.subscriptions(id);


--
-- Name: reminder_message_logs reminder_message_logs_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reminder_message_logs
    ADD CONSTRAINT reminder_message_logs_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: reminder_message_logs reminder_message_logs_user_inactivity_reminder_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reminder_message_logs
    ADD CONSTRAINT reminder_message_logs_user_inactivity_reminder_id_fkey FOREIGN KEY (user_inactivity_reminder_id) REFERENCES public.user_inactivity_reminders(id);


--
-- Name: subscriptions subscriptions_tariff_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT subscriptions_tariff_id_fkey FOREIGN KEY (tariff_id) REFERENCES public.tariffs(id);


--
-- Name: subscriptions subscriptions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT subscriptions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: tariff_groups tariff_groups_terminology_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tariff_groups
    ADD CONSTRAINT tariff_groups_terminology_id_fkey FOREIGN KEY (terminology_id) REFERENCES public.message_terminologies(id);


--
-- Name: tariff_mode tariff_mode_mode_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tariff_mode
    ADD CONSTRAINT tariff_mode_mode_id_fkey FOREIGN KEY (mode_id) REFERENCES public.modes(id);


--
-- Name: tariff_mode tariff_mode_tariff_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tariff_mode
    ADD CONSTRAINT tariff_mode_tariff_id_fkey FOREIGN KEY (tariff_id) REFERENCES public.tariffs(id);


--
-- Name: tariffs tariffs_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tariffs
    ADD CONSTRAINT tariffs_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.tariff_groups(id);


--
-- Name: user_identities user_identities_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_identities
    ADD CONSTRAINT user_identities_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_inactivity_reminders user_inactivity_reminders_dialog_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_inactivity_reminders
    ADD CONSTRAINT user_inactivity_reminders_dialog_id_fkey FOREIGN KEY (dialog_id) REFERENCES public.users_dialogs(id);


--
-- Name: user_inactivity_reminders user_inactivity_reminders_reminder_config_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_inactivity_reminders
    ADD CONSTRAINT user_inactivity_reminders_reminder_config_id_fkey FOREIGN KEY (reminder_config_id) REFERENCES public.reminder_configs(id);


--
-- Name: user_inactivity_reminders user_inactivity_reminders_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_inactivity_reminders
    ADD CONSTRAINT user_inactivity_reminders_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: user_mode_access user_mode_access_mode_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_mode_access
    ADD CONSTRAINT user_mode_access_mode_id_fkey FOREIGN KEY (mode_id) REFERENCES public.modes(id);


--
-- Name: user_mode_access user_mode_access_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_mode_access
    ADD CONSTRAINT user_mode_access_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: users users_current_dialog_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_current_dialog_fkey FOREIGN KEY (current_dialog) REFERENCES public.users_dialogs(id) ON DELETE SET NULL;


--
-- Name: users users_current_mode_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_current_mode_fkey FOREIGN KEY (current_mode) REFERENCES public.modes(id);


--
-- Name: users_dialogs users_dialogs_mode_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users_dialogs
    ADD CONSTRAINT users_dialogs_mode_id_fkey FOREIGN KEY (mode_id) REFERENCES public.modes(id);


--
-- Name: users_dialogs users_dialogs_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users_dialogs
    ADD CONSTRAINT users_dialogs_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: cookie_consents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cookie_consents (
    id bigint NOT NULL,
    consent_id text NOT NULL,
    user_id bigint,
    granted_at timestamp with time zone DEFAULT now() NOT NULL,
    source_path text,
    user_agent text,
    ip_hash text
);


CREATE SEQUENCE public.cookie_consents_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.cookie_consents_id_seq OWNED BY public.cookie_consents.id;


ALTER TABLE ONLY public.cookie_consents ALTER COLUMN id SET DEFAULT nextval('public.cookie_consents_id_seq'::regclass);


ALTER TABLE ONLY public.cookie_consents
    ADD CONSTRAINT cookie_consents_pkey PRIMARY KEY (id);


ALTER TABLE ONLY public.cookie_consents
    ADD CONSTRAINT cookie_consents_consent_id_key UNIQUE (consent_id);


CREATE INDEX cookie_consents_granted_at_idx ON public.cookie_consents USING btree (granted_at DESC);


CREATE INDEX cookie_consents_user_id_idx ON public.cookie_consents USING btree (user_id) WHERE (user_id IS NOT NULL);


ALTER TABLE ONLY public.cookie_consents
    ADD CONSTRAINT cookie_consents_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: site_content; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.site_content (
    key text NOT NULL,
    value jsonb DEFAULT '{}'::jsonb NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_by bigint
);


ALTER TABLE ONLY public.site_content
    ADD CONSTRAINT site_content_pkey PRIMARY KEY (key);


ALTER TABLE ONLY public.site_content
    ADD CONSTRAINT site_content_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: site_media; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.site_media (
    id bigint NOT NULL,
    scope text DEFAULT 'blog'::text NOT NULL,
    filename text NOT NULL,
    content_type text NOT NULL,
    size_bytes integer NOT NULL,
    sha256 text NOT NULL,
    data bytea NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    created_by bigint,
    CONSTRAINT site_media_size_bytes_check CHECK (((size_bytes > 0) AND (size_bytes <= 5242880)))
);

CREATE SEQUENCE public.site_media_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.site_media_id_seq OWNED BY public.site_media.id;

ALTER TABLE ONLY public.site_media ALTER COLUMN id SET DEFAULT nextval('public.site_media_id_seq'::regclass);

ALTER TABLE ONLY public.site_media
    ADD CONSTRAINT site_media_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX site_media_scope_sha256_idx ON public.site_media USING btree (scope, sha256);

CREATE INDEX site_media_scope_created_idx ON public.site_media USING btree (scope, created_at DESC);

ALTER TABLE ONLY public.site_media
    ADD CONSTRAINT site_media_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;


CREATE INDEX notification_contacts_user_idx ON public.notification_contacts USING btree (user_id, channel);

CREATE INDEX notification_consents_user_idx ON public.notification_channel_consents USING btree (user_id, channel, consent_type);

CREATE INDEX notification_inbox_user_idx ON public.notification_inbox USING btree (user_id, status, created_at DESC);

CREATE INDEX notification_delivery_user_idx ON public.notification_delivery_attempts USING btree (user_id, channel, created_at DESC);

ALTER TABLE ONLY public.notification_contacts
    ADD CONSTRAINT notification_contacts_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.notification_channel_consents
    ADD CONSTRAINT notification_channel_consents_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.notification_reachability
    ADD CONSTRAINT notification_reachability_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.notification_inbox
    ADD CONSTRAINT notification_inbox_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.notification_inbox
    ADD CONSTRAINT notification_inbox_template_key_fkey FOREIGN KEY (template_key) REFERENCES public.notification_templates(key) ON DELETE SET NULL;

ALTER TABLE ONLY public.notification_delivery_attempts
    ADD CONSTRAINT notification_delivery_attempts_inbox_id_fkey FOREIGN KEY (inbox_id) REFERENCES public.notification_inbox(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.notification_delivery_attempts
    ADD CONSTRAINT notification_delivery_attempts_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

INSERT INTO public.notification_templates (key, name, consent_type, default_channel, title, body)
VALUES
  ('service.reminder_30m', 'Напоминание за 30 минут', 'service', 'in_site', 'Напомнить за 30 минут', 'Мы напомним перед важным упражнением или встречей.'),
  ('service.access_granted', 'Доступ включён', 'service', 'in_site', 'Доступ включён', 'Можно вернуться в чат и продолжить работу.'),
  ('marketing.product_news', 'Новости продукта', 'marketing', 'email', 'Что нового в Mindstrata', 'Коротко расскажем о новых режимах и сценариях.');


--
-- Name: yookassa_webhook_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.yookassa_webhook_events (
    id bigint NOT NULL,
    event text NOT NULL,
    object_id text NOT NULL,
    received_at timestamp with time zone DEFAULT now() NOT NULL,
    payload jsonb,
    remote_ip text,
    process_status text NOT NULL DEFAULT 'pending',
    processed_at timestamp with time zone
);

CREATE SEQUENCE public.yookassa_webhook_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.yookassa_webhook_events_id_seq OWNED BY public.yookassa_webhook_events.id;

ALTER TABLE ONLY public.yookassa_webhook_events ALTER COLUMN id SET DEFAULT nextval('public.yookassa_webhook_events_id_seq'::regclass);

ALTER TABLE ONLY public.yookassa_webhook_events
    ADD CONSTRAINT yookassa_webhook_events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.yookassa_webhook_events
    ADD CONSTRAINT yookassa_webhook_events_event_object_id_key UNIQUE (event, object_id);

CREATE INDEX yookassa_webhook_events_received_idx ON public.yookassa_webhook_events USING btree (received_at DESC);


--
-- Name: ai_gateways; Type: TABLE; Schema: public; Owner: -
-- Синхронизировано с apps/api/sql/20260917_120000_ai_gateways.sql.
-- Сиды сюда НЕ переносятся: миграция берёт их из system_settings, а
-- интеграционные тесты заводят нужные строки сами и не должны стартовать с
-- чужими тремя шлюзами в реестре.
--

CREATE TABLE public.ai_gateways (
    id text NOT NULL,
    title text NOT NULL,
    protocol text NOT NULL,
    base_url text DEFAULT ''::text NOT NULL,
    relay_url text DEFAULT ''::text NOT NULL,
    use_relay boolean DEFAULT false NOT NULL,
    api_key text DEFAULT ''::text NOT NULL,
    default_model text DEFAULT ''::text NOT NULL,
    priority integer DEFAULT 100 NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    processing_country text DEFAULT ''::text NOT NULL,
    CONSTRAINT ai_gateways_id_check CHECK ((id ~ '^[a-z0-9][a-z0-9_-]{1,31}$'::text)),
    CONSTRAINT ai_gateways_processing_country_check CHECK ((processing_country ~ '^([A-Z]{2})?$'::text)),
    CONSTRAINT ai_gateways_protocol_check CHECK ((protocol = ANY (ARRAY['openai'::text, 'anthropic'::text, 'gemini'::text])))
);

ALTER TABLE ONLY public.ai_gateways
    ADD CONSTRAINT ai_gateways_pkey PRIMARY KEY (id);

CREATE INDEX idx_ai_gateways_priority ON public.ai_gateways USING btree (priority, id) WHERE (archived_at IS NULL);


--
-- Name: consent_records; Type: TABLE; Schema: public; Owner: -
-- Synced with apps/api/sql/20260930_120000_data_protection.sql.
--

CREATE TABLE public.consent_records (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    document text NOT NULL,
    version text NOT NULL,
    text_sha256 text NOT NULL,
    granted_at timestamp with time zone DEFAULT now() NOT NULL,
    withdrawn_at timestamp with time zone,
    ip_hash text,
    CONSTRAINT consent_records_document_check CHECK ((document = ANY (ARRAY['personal_data'::text, 'health_data'::text, 'cross_border_transfer'::text, 'terms_of_service'::text]))),
    CONSTRAINT consent_records_version_check CHECK ((version ~ '^[A-Za-z0-9._-]{1,64}$'::text)),
    CONSTRAINT consent_records_text_sha256_check CHECK ((text_sha256 ~ '^[0-9a-f]{64}$'::text))
);

CREATE SEQUENCE public.consent_records_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.consent_records_id_seq OWNED BY public.consent_records.id;

ALTER TABLE ONLY public.consent_records ALTER COLUMN id SET DEFAULT nextval('public.consent_records_id_seq'::regclass);

ALTER TABLE ONLY public.consent_records
    ADD CONSTRAINT consent_records_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.consent_records
    ADD CONSTRAINT consent_records_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

CREATE INDEX idx_consent_records_active ON public.consent_records USING btree (user_id, document) WHERE (withdrawn_at IS NULL);


--
-- Name: game_results; Type: TABLE; Schema: public; Owner: -
-- Синхронизировано с apps/api/sql/20260918_100000_game_results.sql.
--

CREATE TABLE public.game_results (
    id bigint NOT NULL,
    note text DEFAULT ''::text NOT NULL,
    mate text DEFAULT ''::text NOT NULL,
    attempts integer DEFAULT 1 NOT NULL,
    game text NOT NULL,
    family text NOT NULL,
    name text NOT NULL,
    student_group text NOT NULL,
    score integer DEFAULT 0 NOT NULL,
    correct integer DEFAULT 0 NOT NULL,
    total integer DEFAULT 0 NOT NULL,
    best_combo integer DEFAULT 0 NOT NULL,
    lives_left integer DEFAULT 0 NOT NULL,
    duration_ms bigint DEFAULT 0 NOT NULL,
    answers jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE SEQUENCE public.game_results_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.game_results_id_seq OWNED BY public.game_results.id;

ALTER TABLE ONLY public.game_results ALTER COLUMN id SET DEFAULT nextval('public.game_results_id_seq'::regclass);

ALTER TABLE ONLY public.game_results
    ADD CONSTRAINT game_results_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX uq_game_results_student ON public.game_results
    USING btree (game, lower(TRIM(BOTH FROM family)), lower(TRIM(BOTH FROM name)), lower(TRIM(BOTH FROM student_group)));

CREATE INDEX idx_game_results_game_score ON public.game_results USING btree (game, score DESC);


--
-- PostgreSQL database dump complete
--

--
-- Name: game_tasks; Type: TABLE; Schema: public; Owner: -
-- Синхронизировано с apps/api/sql/20260919_120000_game_tasks.sql и
-- 20260924_120000_game_tasks_quest_kinds.sql (kind, payload).
--

CREATE TABLE public.game_tasks (
    id bigint NOT NULL,
    game text DEFAULT 'praktika-1'::text NOT NULL,
    task text NOT NULL,
    ext_id integer NOT NULL,
    text text NOT NULL,
    answer jsonb NOT NULL,
    lvl integer DEFAULT 1 NOT NULL,
    src text DEFAULT ''::text NOT NULL,
    why text DEFAULT ''::text NOT NULL,
    marks integer[] DEFAULT '{}'::integer[] NOT NULL,
    from_doc boolean DEFAULT false NOT NULL,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    variant integer DEFAULT 0 NOT NULL,
    step integer DEFAULT 0 NOT NULL,
    kind text DEFAULT 'binary'::text NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT game_tasks_kind_valid CHECK ((kind = ANY (ARRAY['binary'::text, 'rate'::text, 'choice'::text]))),
    CONSTRAINT game_tasks_lvl_valid CHECK (((lvl >= 1) AND (lvl <= 3))),
    CONSTRAINT game_tasks_task_valid CHECK ((task = ANY (ARRAY['1.1'::text, '1.2'::text, '1.3'::text, '1.4'::text, '2.1'::text, '2.2'::text])))
);

CREATE SEQUENCE public.game_tasks_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.game_tasks_id_seq OWNED BY public.game_tasks.id;

ALTER TABLE ONLY public.game_tasks ALTER COLUMN id SET DEFAULT nextval('public.game_tasks_id_seq'::regclass);

ALTER TABLE ONLY public.game_tasks
    ADD CONSTRAINT game_tasks_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX uq_game_tasks_ext ON public.game_tasks USING btree (game, task, ext_id);

CREATE INDEX idx_game_tasks_task ON public.game_tasks USING btree (game, task) WHERE (archived_at IS NULL);

--
-- Name: tunnel_machines; Type: TABLE; Schema: public; Owner: -
-- Синхронизировано с apps/api/sql/20260921_100000_tunnel_machines.sql.
--

CREATE TABLE public.tunnel_machines (
    id bigint NOT NULL,
    name text NOT NULL,
    port integer NOT NULL,
    public_key text NOT NULL,
    fingerprint text NOT NULL,
    os text DEFAULT ''::text NOT NULL,
    note text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    last_seen_at timestamp with time zone,
    revoked_at timestamp with time zone,
    CONSTRAINT tunnel_machines_port_range CHECK (((port >= 22001) AND (port <= 22099)))
);

CREATE SEQUENCE public.tunnel_machines_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.tunnel_machines_id_seq OWNED BY public.tunnel_machines.id;

ALTER TABLE ONLY public.tunnel_machines ALTER COLUMN id SET DEFAULT nextval('public.tunnel_machines_id_seq'::regclass);

ALTER TABLE ONLY public.tunnel_machines
    ADD CONSTRAINT tunnel_machines_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX uq_tunnel_machines_port ON public.tunnel_machines USING btree (port) WHERE (revoked_at IS NULL);

CREATE UNIQUE INDEX uq_tunnel_machines_fingerprint ON public.tunnel_machines USING btree (fingerprint) WHERE (revoked_at IS NULL);

--
-- Name: game_sessions; Type: TABLE; Schema: public; Owner: -
-- Синхронизировано с apps/api/sql/20260922_100000_game_sessions.sql.
--

CREATE TABLE public.game_sessions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    game text DEFAULT 'praktika-1'::text NOT NULL,
    task text NOT NULL,
    family text NOT NULL,
    name text NOT NULL,
    student_group text NOT NULL,
    item_ids integer[] NOT NULL,
    answers jsonb DEFAULT '[]'::jsonb NOT NULL,
    score integer DEFAULT 0 NOT NULL,
    correct integer DEFAULT 0 NOT NULL,
    armor integer DEFAULT 100 NOT NULL,
    ip_hash text DEFAULT ''::text NOT NULL,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    last_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    finished_at timestamp with time zone,
    variant integer DEFAULT 0 NOT NULL,
    mate text DEFAULT ''::text NOT NULL,
    client_log jsonb DEFAULT '[]'::jsonb NOT NULL,
    CONSTRAINT game_sessions_task_valid CHECK ((task = ANY (ARRAY['1.1'::text, '1.2'::text, '1.3'::text, '1.4'::text, '2.1'::text, '2.2'::text])))
);

ALTER TABLE ONLY public.game_sessions ADD CONSTRAINT game_sessions_pkey PRIMARY KEY (id);

CREATE INDEX idx_game_sessions_student ON public.game_sessions USING btree (lower(TRIM(BOTH FROM family)), lower(TRIM(BOTH FROM name)), lower(TRIM(BOTH FROM student_group)), task);

CREATE INDEX idx_game_sessions_started ON public.game_sessions USING btree (started_at DESC);

--
-- Name: security_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.security_events (
    id bigint NOT NULL,
    kind text NOT NULL,
    severity text DEFAULT 'warn'::text NOT NULL,
    ip_hash text DEFAULT ''::text NOT NULL,
    path text DEFAULT ''::text NOT NULL,
    detail jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE SEQUENCE public.security_events_id_seq
    START WITH 1 INCREMENT BY 1 NO MINVALUE NO MAXVALUE CACHE 1;

ALTER SEQUENCE public.security_events_id_seq OWNED BY public.security_events.id;

ALTER TABLE ONLY public.security_events ALTER COLUMN id SET DEFAULT nextval('public.security_events_id_seq'::regclass);

ALTER TABLE ONLY public.security_events ADD CONSTRAINT security_events_pkey PRIMARY KEY (id);

CREATE INDEX idx_security_events_time ON public.security_events USING btree (created_at DESC);

CREATE INDEX idx_security_events_kind ON public.security_events USING btree (kind, created_at DESC);

--
-- Name: pii_vault; Type: TABLE; Schema: public; Owner: -
-- Synced with apps/api/sql/20260930_130000_pii_vault.sql.
--

CREATE TABLE public.pii_vault (
    user_id bigint NOT NULL,
    kind text NOT NULL,
    alias_no integer NOT NULL,
    value_hmac bytea NOT NULL,
    nonce bytea NOT NULL,
    ciphertext bytea NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    last_used_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT pii_vault_alias_no_check CHECK ((alias_no > 0)),
    CONSTRAINT pii_vault_kind_check CHECK ((kind = ANY (ARRAY['PERSON'::text, 'PHONE'::text, 'EMAIL'::text, 'URL'::text, 'HANDLE'::text, 'ADDRESS'::text, 'PLACE'::text, 'ORG'::text, 'DATE'::text, 'DOCUMENT'::text, 'CARD'::text, 'ACCOUNT'::text])))
);

ALTER TABLE ONLY public.pii_vault ADD CONSTRAINT pii_vault_pkey PRIMARY KEY (user_id, kind, alias_no);

ALTER TABLE ONLY public.pii_vault ADD CONSTRAINT pii_vault_user_id_kind_value_hmac_key UNIQUE (user_id, kind, value_hmac);

ALTER TABLE ONLY public.pii_vault ADD CONSTRAINT pii_vault_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

CREATE INDEX idx_pii_vault_last_used ON public.pii_vault USING btree (last_used_at);
