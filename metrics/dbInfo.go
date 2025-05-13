package metrics

import (
	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
)

type DbInfoGatherer struct {
	logger        logging.Logger
	configuration *config.Config
}

func NewDbInfoGatherer(logger logging.Logger, configuration *config.Config) *DbInfoGatherer {
	return &DbInfoGatherer{
		logger:        logger,
		configuration: configuration,
	}
}

func (DbInfo *DbInfoGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(DbInfo.configuration, DbInfo.logger)
	var row models.MetricValue

	var output []string
	rows, err := models.DB.Query("SHOW GRANTS")
	if err != nil {
		DbInfo.logger.Error(err)
		return err
	}
	for rows.Next() {
		err := rows.Scan(&row.Value)
		if err != nil {
			DbInfo.logger.Error(err)
			return err
		}
		output = append(output, row.Value)
	}
	rows.Close()
	metrics.DB.Info["Grants"] = output

	metrics.DB.Info["Users"] = security_recommendations(DbInfo)

	DbInfo.logger.V(5).Info("CollectMetrics DbInfo ", metrics.DB.Info)
	return nil

}

func security_recommendations(DbInfo *DbInfoGatherer) []models.MetricGroupValue {
	var output_users []models.MetricGroupValue

	var password_column_exists, authstring_column_exists int
	err := models.DB.QueryRow("SELECT 1 FROM information_schema.columns WHERE TABLE_SCHEMA = 'mysql' AND TABLE_NAME = 'user' AND COLUMN_NAME = 'password'").Scan(&password_column_exists)
	if err != nil {
		DbInfo.logger.Error(err)
	}
	err = models.DB.QueryRow("SELECT 1 FROM information_schema.columns WHERE TABLE_SCHEMA = 'mysql' AND TABLE_NAME = 'user' AND COLUMN_NAME = 'authentication_string'").Scan(&authstring_column_exists)
	if err != nil {
		DbInfo.logger.Error(err)
	}
	// DbInfo.logger.Info(password_column_exists, authstring_column_exists)
	PASS_COLUMN_NAME := "password"
	if password_column_exists == 1 && authstring_column_exists == 1 {
		PASS_COLUMN_NAME = "IF(plugin='mysql_native_password', authentication_string, password)"
	} else if authstring_column_exists == 1 {
		PASS_COLUMN_NAME = "authentication_string"
	} else if password_column_exists != 1 {
		return output_users
	}
	DbInfo.logger.Info(PASS_COLUMN_NAME)

	var Username, User, Host, Blank_Password, Password_As_User string
	rows_users, err := models.DB.Query("SELECT CONCAT(QUOTE(user), '\\@', QUOTE(host)), user, host, (" + PASS_COLUMN_NAME + " = '' OR " + PASS_COLUMN_NAME + " IS NULL) as Blank_Password, (CAST(" + PASS_COLUMN_NAME + " as Binary) = PASSWORD(user) OR CAST(" + PASS_COLUMN_NAME + " as Binary) = PASSWORD(UPPER(user)) ) as Password_As_User FROM mysql.user")
	if err != nil || rows_users.Next() == false {
		DbInfo.logger.Error(err)
		rows_users, err = models.DB.Query("SELECT CONCAT(QUOTE(user), '\\@', QUOTE(host)), user, host, (" + PASS_COLUMN_NAME + " = '' OR " + PASS_COLUMN_NAME + " IS NULL) as Blank_Password, (CAST(" + PASS_COLUMN_NAME + " as Binary) = CONCAT('*',UPPER(SHA1(UNHEX(SHA1(user))))) OR CAST(" + PASS_COLUMN_NAME + " as Binary) = CONCAT('*',UPPER(SHA1(UNHEX(SHA1(UPPER(user)))))) ) as Password_As_User FROM mysql.user")
		if err != nil {
			DbInfo.logger.Error(err)
			return output_users
		}
		defer rows_users.Close()
		for rows_users.Next() {
			err := rows_users.Scan(&Username, &User, &Host, &Blank_Password, &Password_As_User)
			if err != nil {
				DbInfo.logger.Error(err)
				continue
			}
			output_users = append(output_users, models.MetricGroupValue{"Username": Username, "User": User, "Host": Host, "Blank_Password": Blank_Password, "Password_As_User": Password_As_User})
		}
	} else {
		defer rows_users.Close()
		err := rows_users.Scan(&Username, &User, &Host, &Blank_Password, &Password_As_User)
		if err != nil {
			DbInfo.logger.Error(err)
		} else {
			output_users = append(output_users, models.MetricGroupValue{"Username": Username, "User": User, "Host": Host, "Blank_Password": Blank_Password, "Password_As_User": Password_As_User})
		}
		for rows_users.Next() {
			err := rows_users.Scan(&Username, &User, &Host, &Blank_Password, &Password_As_User)
			if err != nil {
				DbInfo.logger.Error(err)
				continue
			}
			output_users = append(output_users, models.MetricGroupValue{"Username": Username, "User": User, "Host": Host, "Blank_Password": Blank_Password, "Password_As_User": Password_As_User})
		}
	}
	return output_users
}
