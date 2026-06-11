-- Modify "core_characters" table
ALTER TABLE "core_characters" ADD COLUMN "version" bigint NOT NULL DEFAULT 1;
