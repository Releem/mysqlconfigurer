package metrics

import (
	"strings"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
	"github.com/hashicorp/go-version"
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

	metrics.DB.Info["UsersSecurityCheck"] = users_security_check(DbInfo, metrics)

	DbInfo.logger.V(5).Info("CollectMetrics DbInfo ", metrics.DB.Info)
	return nil

}

func users_security_check(DbInfo *DbInfoGatherer, metrics *models.Metrics) []models.MetricGroupValue {
	var output_users, users_check []models.MetricGroupValue

	var password_column_exists, authstring_column_exists int

	// New table schema available since mysql-5.7 and mariadb-10.2
	// But need to be checked
	models.DB.QueryRow("SELECT 1 FROM information_schema.columns WHERE TABLE_SCHEMA = 'mysql' AND TABLE_NAME = 'user' AND COLUMN_NAME = 'password'").Scan(&password_column_exists)
	models.DB.QueryRow("SELECT 1 FROM information_schema.columns WHERE TABLE_SCHEMA = 'mysql' AND TABLE_NAME = 'user' AND COLUMN_NAME = 'authentication_string'").Scan(&authstring_column_exists)
	PASS_COLUMN_NAME := "password"
	ver_current, _ := version.NewVersion(metrics.DB.Info["Version"].(string))
	ver_mariadb, _ := version.NewVersion("10.2.0")
	ver_mysql, _ := version.NewVersion("5.7.0")

	if (strings.Contains(metrics.DB.Conf.Variables["version"].(string), "MariaDB") && ver_current.GreaterThan(ver_mariadb)) || (!strings.Contains(metrics.DB.Conf.Variables["version"].(string), "MariaDB") && ver_current.GreaterThan(ver_mysql)) {
		if password_column_exists == 1 && authstring_column_exists == 1 {
			PASS_COLUMN_NAME = "IF(plugin='mysql_native_password', authentication_string, password)"
		} else if authstring_column_exists == 1 {
			PASS_COLUMN_NAME = "authentication_string"
		} else if password_column_exists != 1 {
			DbInfo.logger.Info("DEBUG: Skipped due to none of known auth columns exists")
			return output_users
		}
	}
	DbInfo.logger.Info("DEBUG: Password column = ", PASS_COLUMN_NAME)

	var Username, User, Host, Password_As_User string
	rows_users, err := models.DB.Query("SELECT CONCAT(QUOTE(user), '@', QUOTE(host)), user, host, (CAST(" + PASS_COLUMN_NAME + " as Binary) = PASSWORD(user) OR CAST(" + PASS_COLUMN_NAME + " as Binary) = PASSWORD(UPPER(user)) ) as Password_As_User FROM mysql.user")
	if err != nil || !rows_users.Next() {
		if err != nil {
			if strings.Contains(err.Error(), "Error 1064 (42000): You have an error in your SQL syntax") {
				DbInfo.logger.Info("DEBUG: PASSWORD() function is not supported. Try another query...")
			} else {
				DbInfo.logger.Error(err)
			}
		} else {
			DbInfo.logger.Info("DEBUG: Plugin validate_password is activated. Try another query...")
		}
		rows_users, err = models.DB.Query("SELECT CONCAT(QUOTE(user), '@', QUOTE(host)), user, host, (CAST(" + PASS_COLUMN_NAME + " as Binary) = CONCAT('*',UPPER(SHA1(UNHEX(SHA1(user))))) OR CAST(" + PASS_COLUMN_NAME + " as Binary) = CONCAT('*',UPPER(SHA1(UNHEX(SHA1(UPPER(user)))))) ) as Password_As_User FROM mysql.user")
		if err != nil {
			DbInfo.logger.Error(err)
		} else {
			defer rows_users.Close()
			for rows_users.Next() {
				err := rows_users.Scan(&Username, &User, &Host, &Password_As_User)
				if err != nil {
					DbInfo.logger.Error(err)
				} else {
					output_users = append(output_users, models.MetricGroupValue{"Username": Username, "User": User, "Host": Host, "Password_As_User": Password_As_User})
				}
			}
		}
	} else {
		defer rows_users.Close()
		err := rows_users.Scan(&Username, &User, &Host, &Password_As_User)
		if err != nil {
			DbInfo.logger.Error(err)
		} else {
			output_users = append(output_users, models.MetricGroupValue{"Username": Username, "User": User, "Host": Host, "Password_As_User": Password_As_User})
		}
		for rows_users.Next() {
			err := rows_users.Scan(&Username, &User, &Host, &Password_As_User)
			if err != nil {
				DbInfo.logger.Error(err)
			} else {
				output_users = append(output_users, models.MetricGroupValue{"Username": Username, "User": User, "Host": Host, "Password_As_User": Password_As_User})
			}
		}
	}

	output_user_blank_password := make(models.MetricGroupValue)
	rows_users, err = models.DB.Query("SELECT CONCAT(QUOTE(user), '@', QUOTE(host)) FROM mysql.global_priv WHERE ( user != '' AND JSON_CONTAINS(Priv, '\"mysql_native_password\"', '$.plugin') AND JSON_CONTAINS(Priv, '\"\"', '$.authentication_string') AND NOT JSON_CONTAINS(Priv, 'true', '$.account_locked'))")
	if err != nil {
		if strings.Contains(err.Error(), "Error 1146 (42S02): Table 'mysql.global_priv' doesn't exist") {
			DbInfo.logger.Info("DEBUG: Not MariaDB, try another query...")
		} else {
			DbInfo.logger.Error(err)
		}
		rows_users, err = models.DB.Query("SELECT CONCAT(QUOTE(user), '@', QUOTE(host)) FROM mysql.user WHERE (" + PASS_COLUMN_NAME + " = '' OR " + PASS_COLUMN_NAME + " IS NULL) AND user != '' /*!50501 AND plugin NOT IN ('auth_socket', 'unix_socket', 'win_socket', 'auth_pam_compat') */  /*!80000 AND account_locked = 'N' AND password_expired = 'N' */")
		if err != nil {
			DbInfo.logger.Error(err)
		} else {
			defer rows_users.Close()
			for rows_users.Next() {
				err := rows_users.Scan(&Username)
				if err != nil {
					DbInfo.logger.Error(err)
				} else {
					output_user_blank_password[Username] = 1
				}
			}
		}
	} else {
		defer rows_users.Close()
		for rows_users.Next() {
			err := rows_users.Scan(&Username)
			if err != nil {
				DbInfo.logger.Error(err)
			} else {
				output_user_blank_password[Username] = 1
			}
		}
	}

	for i, user := range output_users {
		_, ok := output_user_blank_password[user["Username"].(string)]

		if ok && user["User"].(string) != "mariadb.sys" && user["User"].(string) != "rdsadmin" {
			output_users[i]["Blank_Password"] = 1
		} else {
			output_users[i]["Blank_Password"] = 0
		}
	}

	for _, user := range output_users {
		remoteConnRoot := 0
		anonymousUser := 0
		if user["User"].(string) == "root" && user["Host"].(string) != "localhost" && user["Host"].(string) != "127.0.0.1" && user["Host"].(string) != "::1" {
			remoteConnRoot = 1
		}

		if strings.TrimSpace(user["User"].(string)) == "" {
			anonymousUser = 1
		}
		users_check = append(users_check, models.MetricGroupValue{"Blank_Password": user["Blank_Password"], "Password_As_User": user["Password_As_User"], "Remote_Conn_Root": remoteConnRoot, "Anonymous_User": anonymousUser})
	}
	return users_check
}
