package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/sirupsen/logrus"
)

const (
	sqlDeleteClones = `
                DELETE FROM clones
                WHERE compose_id in (
                    SELECT job_id
                    FROM composes
                    WHERE created_at < $1
                )`
	sqlDeleteComposes = `
                DELETE FROM composes
                WHERE created_at < $1`
	sqlExpiredClonesCount = `
                SELECT COUNT(*) FROM clones
                WHERE compose_id in (
                    SELECT job_id
                    FROM composes
                    WHERE created_at < $1
                )`
	sqlExpiredComposesCount = `
                SELECT COUNT(*) FROM composes
                WHERE created_at < $1`
	sqlVacuumAnalyze = `
                VACUUM ANALYZE`
	sqlVacuumStats = `
                SELECT relname, pg_size_pretty(pg_total_relation_size(relid)),
                    n_tup_ins, n_tup_upd, n_tup_del, n_live_tup, n_dead_tup,
                    vacuum_count, autovacuum_count, analyze_count, autoanalyze_count,
                    last_vacuum, last_autovacuum, last_analyze, last_autoanalyze
                 FROM pg_stat_user_tables`
)

type maintenanceDB struct {
	Conn *pgx.Conn
}

func newDB(dbURL string) (maintenanceDB, error) {
	conn, err := pgx.Connect(context.Background(), dbURL)
	if err != nil {
		return maintenanceDB{}, err
	}

	return maintenanceDB{
		conn,
	}, nil
}

func (d *maintenanceDB) Close() error {
	return d.Conn.Close(context.Background())
}

func (d *maintenanceDB) DeleteClones(emailRetentionDate time.Time) (int64, error) {
	tag, err := d.Conn.Exec(context.Background(), sqlDeleteClones, emailRetentionDate)
	if err != nil {
		return tag.RowsAffected(), fmt.Errorf("Error deleting clones: %v", err)
	}
	return tag.RowsAffected(), nil
}

func (d *maintenanceDB) DeleteComposes(emailRetentionDate time.Time) (int64, error) {
	tag, err := d.Conn.Exec(context.Background(), sqlDeleteComposes, emailRetentionDate)
	if err != nil {
		return tag.RowsAffected(), fmt.Errorf("Error deleting composes: %v", err)
	}
	return tag.RowsAffected(), nil
}

func (d *maintenanceDB) ExpiredClonesCount(emailRetentionDate time.Time) (int64, error) {
	var count int64
	err := d.Conn.QueryRow(context.Background(), sqlExpiredClonesCount, emailRetentionDate).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (d *maintenanceDB) ExpiredComposesCount(emailRetentionDate time.Time) (int64, error) {
	var count int64
	err := d.Conn.QueryRow(context.Background(), sqlExpiredComposesCount, emailRetentionDate).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (d *maintenanceDB) VacuumAnalyze() error {
	_, err := d.Conn.Exec(context.Background(), sqlVacuumAnalyze)
	if err != nil {
		return fmt.Errorf("Error running VACUUM ANALYZE: %v", err)
	}
	return nil
}

func (d *maintenanceDB) LogVacuumStats() (int64, error) {
	rows, err := d.Conn.Query(context.Background(), sqlVacuumStats)
	if err != nil {
		return int64(0), fmt.Errorf("Error querying vacuum stats: %v", err)
	}
	defer rows.Close()

	deleted := int64(0)

	for rows.Next() {
		var relName, relSize string
		var ins, upd, del, live, dead, vc, avc, ac, aac int64
		var lvc, lavc, lan, laan *time.Time

		err = rows.Scan(&relName, &relSize, &ins, &upd, &del, &live, &dead,
			&vc, &avc, &ac, &aac,
			&lvc, &lavc, &lan, &laan)
		if err != nil {
			return int64(0), err
		}

		logrus.WithFields(logrus.Fields{
			"table_name":        relName,
			"table_size":        relSize,
			"tuples_inserted":   ins,
			"tuples_updated":    upd,
			"tuples_deleted":    del,
			"tuples_live":       live,
			"tuples_dead":       dead,
			"vacuum_count":      vc,
			"autovacuum_count":  avc,
			"last_vacuum":       lvc,
			"last_autovacuum":   lavc,
			"analyze_count":     ac,
			"autoanalyze_count": aac,
			"last_analyze":      lan,
			"last_autoanalyze":  laan,
		}).Info("Vacuum and analyze stats for table")
	}
	if rows.Err() != nil {
		return int64(0), rows.Err()
	}
	return deleted, nil

}

func DBCleanup(dbURL string, dryRun bool, ClonesRetentionMonths int) error {
	db, err := newDB(dbURL)
	if err != nil {
		return err
	}

	_, err = db.LogVacuumStats()
	if err != nil {
		logrus.Errorf("Error running vacuum stats: %v", err)
	}

	var rowsClones int64
	var rows int64

	emailRetentionDate := time.Now().AddDate(0, ClonesRetentionMonths*-1, 0)

	for {
		if dryRun {
			rowsClones, err = db.ExpiredClonesCount(emailRetentionDate)
			if err != nil {
				logrus.Warningf("Error querying expired clones: %v", err)
			}

			rows, err = db.ExpiredComposesCount(emailRetentionDate)
			if err != nil {
				logrus.Warningf("Error querying expired composes: %v", err)
			}
			logrus.Infof("Dryrun, expired composes count: %d (affecting %d clones)", rows, rowsClones)
			break
		}

		rows, err = db.DeleteClones(emailRetentionDate)
		if err != nil {
			logrus.Errorf("Error deleting clones: %v, %d rows affected", rows, err)
			return err
		}

		rows, err = db.DeleteComposes(emailRetentionDate)
		if err != nil {
			logrus.Errorf("Error deleting composes: %v, %d rows affected", rows, err)
			return err
		}

		err = db.VacuumAnalyze()
		if err != nil {
			logrus.Errorf("Error running vacuum analyze: %v", err)
			return err
		}

		if rows == 0 {
			break
		}

		logrus.Infof("Deleted results for %d", rows)
	}

	_, err = db.LogVacuumStats()
	if err != nil {
		logrus.Errorf("Error running vacuum stats: %v", err)
	}

	return nil
}