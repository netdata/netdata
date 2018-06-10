# -*- coding: utf-8 -*-
# Description:
# Author: Ilya Mashchenko (l2isbad)
# SPDX-License-Identifier: GPL-3.0+

from sys import exc_info

try:
    import MySQLdb

    PY_MYSQL = True
except ImportError:
    try:
        import pymysql as MySQLdb

        PY_MYSQL = True
    except ImportError:
        PY_MYSQL = False

from bases.FrameworkServices.SimpleService import SimpleService


class MySQLService(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.__connection = None
        self.__conn_properties = dict()
        self.extra_conn_properties = dict()
        self.__queries = self.configuration.get('queries', dict())
        self.queries = dict()

    def __connect(self):
        try:
            connection = MySQLdb.connect(connect_timeout=self.update_every, **self.__conn_properties)
        except (MySQLdb.MySQLError, TypeError, AttributeError) as error:
            return None, str(error)
        else:
            return connection, None

    def check(self):
        def get_connection_properties(conf, extra_conf):
            properties = dict()
            if conf.get('user'):
                properties['user'] = conf['user']
            if conf.get('pass'):
                properties['passwd'] = conf['pass']
            if conf.get('socket'):
                properties['unix_socket'] = conf['socket']
            elif conf.get('host'):
                properties['host'] = conf['host']
                properties['port'] = int(conf.get('port', 3306))
            elif conf.get('my.cnf'):
                if MySQLdb.__name__ == 'pymysql':
                    self.error('"my.cnf" parsing is not working for pymysql')
                else:
                    properties['read_default_file'] = conf['my.cnf']
            if isinstance(extra_conf, dict) and extra_conf:
                properties.update(extra_conf)

            return properties or None

        def is_valid_queries_dict(raw_queries, log_error):
            """
            :param raw_queries: dict:
            :param log_error: function:
            :return: dict or None

            raw_queries is valid when: type <dict> and not empty after is_valid_query(for all queries)
            """

            def is_valid_query(query):
                return all([isinstance(query, str),
                            query.startswith(('SELECT', 'select', 'SHOW', 'show'))])

            if hasattr(raw_queries, 'keys') and raw_queries:
                valid_queries = dict([(n, q) for n, q in raw_queries.items() if is_valid_query(q)])
                bad_queries = set(raw_queries) - set(valid_queries)

                if bad_queries:
                    log_error('Removed query(s): {queries}'.format(queries=bad_queries))
                return valid_queries
            else:
                log_error('Unsupported "queries" format. Must be not empty <dict>')
                return None

        if not PY_MYSQL:
            self.error('MySQLdb or PyMySQL module is needed to use mysql.chart.py plugin')
            return False

        # Preference: 1. "queries" from the configuration file 2. "queries" from the module
        self.queries = self.__queries or self.queries
        # Check if "self.queries" exist, not empty and all queries are in valid format
        self.queries = is_valid_queries_dict(self.queries, self.error)
        if not self.queries:
            return None

        # Get connection properties
        self.__conn_properties = get_connection_properties(self.configuration, self.extra_conn_properties)
        if not self.__conn_properties:
            self.error('Connection properties are missing')
            return False

        # Create connection to the database
        self.__connection, error = self.__connect()
        if error:
            self.error('Can\'t establish connection to MySQL: {error}'.format(error=error))
            return False

        try:
            data = self._get_data()
        except Exception as error:
            self.error('_get_data() failed. Error: {error}'.format(error=error))
            return False

        if isinstance(data, dict) and data:
            return True
        self.error("_get_data() returned no data or type is not <dict>")
        return False

    def _get_raw_data(self, description=None):
        """
        Get raw data from MySQL server
        :return: dict: fetchall() or (fetchall(), description)
        """

        if not self.__connection:
            self.__connection, error = self.__connect()
            if error:
                return None

        raw_data = dict()
        queries = dict(self.queries)
        try:
            with self.__connection as cursor:
                for name, query in queries.items():
                    try:
                        cursor.execute(query)
                    except (MySQLdb.ProgrammingError, MySQLdb.OperationalError) as error:
                        if self.__is_error_critical(err_class=exc_info()[0], err_text=str(error)):
                            raise RuntimeError
                        self.error('Removed query: {name}[{query}]. Error: error'.format(name=name,
                                                                                         query=query,
                                                                                         error=error))
                        self.queries.pop(name)
                        continue
                    else:
                        raw_data[name] = (cursor.fetchall(), cursor.description) if description else cursor.fetchall()
            self.__connection.commit()
        except (MySQLdb.MySQLError, RuntimeError, TypeError, AttributeError):
            self.__connection.close()
            self.__connection = None
            return None
        else:
            return raw_data or None

    @staticmethod
    def __is_error_critical(err_class, err_text):
        return err_class == MySQLdb.OperationalError and all(['denied' not in err_text,
                                                              'Unknown column' not in err_text])
