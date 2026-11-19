package main

// denmaStatusUpdatedSQL sets a subscriber's updated_at when their status changes and the update gives no date (denmaFeaturesSQL).
const denmaStatusUpdatedSQL = `
CREATE OR REPLACE FUNCTION denma_status_updated() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at := NOW();
    RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS denma_status_updated ON subscribers;
CREATE TRIGGER denma_status_updated BEFORE UPDATE ON subscribers
    FOR EACH ROW WHEN (OLD.status IS DISTINCT FROM NEW.status AND NEW.updated_at IS NOT DISTINCT FROM OLD.updated_at)
    EXECUTE FUNCTION denma_status_updated();
`

// denmaStatusUpdatedFixSQL dates the blocklisted whose updated_at is before they were blocklisted or opted out (once).
const denmaStatusUpdatedFixSQL = `
UPDATE subscribers s SET updated_at = b.at FROM denma_blocklisted_at b
    WHERE b.subscriber_id = s.id AND s.status = 'blocklisted' AND s.updated_at < b.at;
UPDATE subscribers s SET updated_at = o.created_at FROM denma_optouts o
    WHERE o.email_hash = denma_email_hash(s.email) AND s.status = 'blocklisted' AND s.updated_at < o.created_at;
`
